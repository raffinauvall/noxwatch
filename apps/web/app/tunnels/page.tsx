"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Cable, CircleOff, Play, RotateCcw, Square, Terminal } from "lucide-react";
import { useAuth } from "@/app/providers";
import { DashboardShell } from "@/components/dashboard-shell";
import { StatusPill } from "@/components/status-pill";
import { Button } from "@/components/ui/button";
import { type ServerRecord, type Workspace } from "@/lib/api";
import { localHelper, syncTunnelProfile, type TunnelStatus } from "@/lib/local-helper";

export default function TunnelsPage() {
  const auth = useAuth();
  const router = useRouter();
  const [busy, setBusy] = useState("");
  const [requestError, setRequestError] = useState("");
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: () => auth.request<Workspace[]>("/api/v1/workspaces"), enabled: Boolean(auth.accessToken) });
  const workspace = workspaces.data?.[0];
  const servers = useQuery({ queryKey: ["servers", workspace?.id], queryFn: () => auth.request<ServerRecord[]>(`/api/v1/servers?workspace_id=${workspace?.id}&limit=100`), enabled: Boolean(auth.accessToken && workspace?.id), refetchInterval: 15_000 });
  const tunnels = useQuery({ queryKey: ["local-tunnels"], queryFn: () => localHelper<TunnelStatus[]>("/tunnels"), retry: false, refetchInterval: 3_000 });

  useEffect(() => { if (!auth.loading && !auth.user) router.replace("/login"); }, [auth.loading, auth.user, router]);
  if (auth.loading || !auth.user || workspaces.isLoading || !workspace) return <main className="min-h-screen animate-pulse bg-background" />;

  const serverRows = servers.data ?? [];
  const serverIDs = new Set(serverRows.map((server) => server.id));
  const tunnelRows = (tunnels.data ?? []).filter((profile) => serverIDs.has(profile.server_id ?? profile.id));
  const rows = serverRows.map((server) => {
    const tunnel = tunnelRows.find((profile) => profile.server_id === server.id || profile.id === server.id);
    return { server, tunnel, configured: Boolean(tunnel || (server.ssh_user && server.ssh_host && server.ssh_port && server.tunnel_remote_port)) };
  });
  const configured = rows.filter((row) => row.configured);
  const connected = configured.filter((row) => row.tunnel?.running).length;
  const allRunning = configured.length > 0 && connected === configured.length;

  async function act(server: ServerRecord, action: "start" | "stop" | "terminal") {
    setBusy(`${action}:${server.id}`);
    setRequestError("");
    try {
      const current = tunnelRows.find((profile) => profile.server_id === server.id || profile.id === server.id);
      const id = current?.id ?? await syncTunnelProfile(server, tunnelRows, auth.request);
      if (!id) throw new Error("Configure this server's SSH target from its detail page first.");
      if (action === "stop") {
        await localHelper("/tunnels/stop", "POST", { id });
        await auth.request(`/api/v1/servers/${server.id}/disconnect`, { method: "POST" });
      } else {
        await localHelper(action === "start" ? "/tunnels/start" : "/terminal", "POST", { id });
      }
      await Promise.all([tunnels.refetch(), servers.refetch()]);
    } catch (error) {
      setRequestError(error instanceof Error ? error.message : "Tunnel action failed.");
    } finally {
      setBusy("");
    }
  }

  async function toggleAll() {
    setBusy("all");
    setRequestError("");
    try {
      if (allRunning) {
        await localHelper("/tunnels/stop-all", "POST", { ids: configured.flatMap((row) => row.tunnel ? [row.tunnel.id] : []) });
        for (const row of configured) await auth.request(`/api/v1/servers/${row.server.id}/disconnect`, { method: "POST" });
      } else {
        const ids: string[] = [];
        for (const row of configured) {
          const id = await syncTunnelProfile(row.server, tunnelRows, auth.request);
          if (id) ids.push(id);
        }
        if (configured.length === 0) throw new Error("No tunnel profiles are configured.");
        await localHelper("/tunnels/start-all", "POST", { ids });
      }
      await Promise.all([tunnels.refetch(), servers.refetch()]);
    } catch (error) {
      setRequestError(error instanceof Error ? error.message : "Tunnel action failed.");
    } finally {
      setBusy("");
    }
  }

  return <DashboardShell workspace={workspace} title="Tunnels" description="Manage local reverse SSH connections" action={<Button variant="secondary" disabled={busy !== ""} onClick={toggleAll}>{allRunning ? <Square className="h-4 w-4" /> : <Cable className="h-4 w-4" />}{busy === "all" ? "Working..." : allRunning ? `Stop all (${connected})` : `Start all (${connected}/${configured.length})`}</Button>}>
    <div className="mx-auto grid max-w-7xl gap-5 px-5 py-6">
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Summary label="Configured" value={configured.length} />
        <Summary label="Connected" value={connected} tone="text-accent" />
        <Summary label="Disconnected" value={configured.length - connected} tone="text-danger" />
        <Summary label="Not configured" value={rows.length - configured.length} />
      </section>

      {requestError && <section className="flex items-center justify-between gap-4 border-l-2 border-danger bg-danger/5 px-4 py-3 text-sm" role="alert"><span>{requestError}</span><button className="text-accent hover:underline" onClick={() => { setRequestError(""); void tunnels.refetch(); }}>Retry</button></section>}
      {tunnels.isError && <section className="flex items-center gap-3 border-l-2 border-warning bg-warning/5 px-4 py-3 text-sm"><CircleOff className="h-4 w-4 text-warning" /><span>Local helper is unavailable. Run <code>make local-helper-install</code>, then retry.</span><button className="ml-auto text-accent hover:underline" onClick={() => tunnels.refetch()}><RotateCcw className="h-4 w-4" /></button></section>}

      {servers.isLoading ? <div className="h-80 animate-pulse rounded-lg bg-panel" /> : servers.isError ? <div className="grid h-72 place-items-center rounded-lg border border-panel-border"><Button variant="secondary" onClick={() => servers.refetch()}><RotateCcw className="h-4 w-4" />Retry servers</Button></div> : rows.length === 0 ? <div className="grid h-72 place-items-center rounded-lg border border-dashed border-panel-border text-sm text-muted">No servers enrolled.</div> :
        <section className="overflow-hidden rounded-lg border border-panel-border bg-panel">
          <div className="hidden grid-cols-[minmax(180px,1fr)_minmax(190px,1fr)_120px_110px_auto] gap-4 border-b border-panel-border px-4 py-3 text-xs font-medium text-muted md:grid"><span>Server</span><span>SSH target</span><span>Tunnel</span><span>Agent</span><span className="text-right">Actions</span></div>
          <div className="divide-y divide-panel-border">{rows.map(({ server, tunnel, configured: ready }) => {
            const target = tunnel?.target ?? (server.ssh_user && server.ssh_host ? `${server.ssh_user}@${server.ssh_host}` : "Not configured");
            const port = tunnel?.port ?? server.ssh_port;
            const working = busy.endsWith(server.id);
            return <article key={server.id} className="grid gap-4 px-4 py-4 md:grid-cols-[minmax(180px,1fr)_minmax(190px,1fr)_120px_110px_auto] md:items-center">
              <div className="min-w-0"><Link href={`/servers/${server.id}`} className="truncate text-sm font-semibold hover:text-accent">{server.name}</Link><p className="mt-1 truncate text-xs text-muted">{server.hostname || "Awaiting identity"}</p></div>
              <div className="min-w-0 font-mono text-xs"><p className="truncate">{target}</p><p className="mt-1 text-muted">{port ? `SSH ${port} · remote ${tunnel?.remote_port ?? server.tunnel_remote_port}` : "Open server detail to configure"}</p></div>
              <span className={`w-fit rounded-full border px-2.5 py-1 text-xs ${tunnel?.running ? "border-accent/40 bg-accent/10 text-accent" : ready ? "border-danger/40 bg-danger/10 text-danger" : "border-panel-border text-muted"}`}>{tunnel?.running ? "Connected" : ready ? "Disconnected" : "Unconfigured"}</span>
              <StatusPill status={server.status} />
              <div className="flex flex-wrap gap-2 md:justify-end">
                {ready && <Button title="Open SSH terminal" variant="secondary" className="h-9 px-3" disabled={working} onClick={() => act(server, "terminal")}><Terminal className="h-4 w-4" /><span className="md:hidden xl:inline">Terminal</span></Button>}
                {ready ? <Button variant="secondary" className="h-9 px-3" disabled={working} onClick={() => act(server, tunnel?.running ? "stop" : "start")}>{tunnel?.running ? <Square className="h-4 w-4" /> : <Play className="h-4 w-4" />}{working ? "Working..." : tunnel?.running ? "Stop" : "Start"}</Button> : <Link href={`/servers/${server.id}`} className="inline-flex h-9 items-center rounded-md border border-panel-border px-3 text-xs hover:bg-background">Configure</Link>}
              </div>
            </article>;
          })}</div>
        </section>}
    </div>
  </DashboardShell>;
}

function Summary({ label, value, tone = "" }: { label: string; value: number; tone?: string }) {
  return <div className="rounded-lg border border-panel-border bg-panel p-4"><p className="text-sm text-muted">{label}</p><p className={`mt-2 text-3xl font-semibold ${tone}`}>{value}</p></div>;
}

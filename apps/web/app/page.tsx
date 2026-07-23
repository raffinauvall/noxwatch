"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowUpRight, Bell, Cable, Plus, RotateCcw, Server, ShieldCheck, Square } from "lucide-react";
import { useAuth } from "@/app/providers";
import { type AlertEvent, type ServerRecord, type Workspace } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { StatusPill } from "@/components/status-pill";
import { DashboardShell } from "@/components/dashboard-shell";
import { parseSSE } from "@/lib/sse.mjs";
import { localHelper, type TunnelStatus } from "@/lib/local-helper";

export default function Home() {
  const router = useRouter();
  const auth = useAuth();
	const queryClient = useQueryClient();
  const [tunnelAction, setTunnelAction] = useState(false);
  const [tunnelError, setTunnelError] = useState("");
  const workspaces = useQuery({
    queryKey: ["workspaces"],
    queryFn: () => auth.request<Workspace[]>("/api/v1/workspaces"),
    enabled: Boolean(auth.accessToken),
  });
  const workspace = workspaces.data?.[0];
  const servers = useQuery({
    queryKey: ["servers", workspace?.id],
    queryFn: () => auth.request<ServerRecord[]>(`/api/v1/servers?workspace_id=${workspace?.id}`),
    enabled: Boolean(auth.accessToken && workspace?.id),
    refetchInterval: 20_000,
  });
  const alerts = useQuery({ queryKey: ["workspace-alerts", workspace?.id], queryFn: () => auth.request<AlertEvent[]>(`/api/v1/alerts?workspace_id=${workspace?.id}`), enabled: Boolean(auth.accessToken && workspace?.id), refetchInterval: 30_000 });
  const tunnels = useQuery({ queryKey: ["local-tunnels"], queryFn: () => localHelper<TunnelStatus[]>("/tunnels"), retry: false, refetchInterval: 3_000 });

  useEffect(() => {
	if (!auth.accessToken || !workspace?.id) return;
	const controller = new AbortController();
	const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
	void (async () => {
		try {
			const response = await fetch(`${apiURL}/api/v1/workspaces/${workspace.id}/events`, { headers: { Authorization: `Bearer ${auth.accessToken}` }, signal: controller.signal });
			if (!response.ok || !response.body) return;
			const reader = response.body.getReader();
			const decoder = new TextDecoder();
			let buffer = "";
			while (!controller.signal.aborted) {
				const { value, done } = await reader.read();
				if (done) break;
				buffer += decoder.decode(value, { stream: true });
				const parsed = parseSSE<Array<Pick<ServerRecord, "id" | "status" | "last_seen_at">>>(buffer);
				buffer = parsed.remaining;
				for (const statuses of parsed.events) {
					queryClient.setQueryData<ServerRecord[]>(["servers", workspace.id], (current) => current?.map((server) => ({ ...server, ...(statuses.find((status) => status.id === server.id) ?? {}) })));
				}
			}
		} catch (error) {
			if (!controller.signal.aborted) console.warn("Live server status stream disconnected.", error);
		}
	})();
	return () => controller.abort();
  }, [auth.accessToken, queryClient, workspace?.id]);

  useEffect(() => {
    if (!auth.loading && !auth.user) router.replace("/login");
  }, [auth.loading, auth.user, router]);

  useEffect(() => {
    if (workspaces.isSuccess && workspaces.data.length === 0) router.replace("/onboarding");
  }, [router, workspaces.data, workspaces.isSuccess]);

  if (auth.loading || !auth.user || workspaces.isLoading || (workspace && servers.isLoading)) return <DashboardSkeleton />;
  if (workspaces.isError || servers.isError) return <DashboardError retry={() => { void workspaces.refetch(); void servers.refetch(); }} />;
  if (!workspaces.data?.length) return <DashboardSkeleton />;

  const currentWorkspace = workspaces.data[0];
  const serverRows = servers.data ?? [];
  const incidents = alerts.data ?? [];
  const activeAlerts = alerts.isError ? "—" : incidents.filter((event) => event.state === "firing" || event.state === "pending").length;
  const tunnelRows = tunnels.data ?? [];
  const runningTunnels = tunnelRows.filter((tunnel) => tunnel.running).length;
  const allTunnelsRunning = tunnelRows.length > 0 && runningTunnels === tunnelRows.length;

  async function toggleTunnels() {
    setTunnelAction(true);
    setTunnelError("");
    try {
      await localHelper(allTunnelsRunning ? "/tunnels/stop-all" : "/tunnels/start-all", "POST");
      await tunnels.refetch();
    } catch (error) {
      setTunnelError(error instanceof Error ? error.message : "Tunnel action failed.");
    } finally {
      setTunnelAction(false);
    }
  }
  const summary = [
    ["Total servers", serverRows.length],
    ["Online", serverRows.filter((server) => server.status === "online").length],
    ["Warning", serverRows.filter((server) => ["warning", "degraded"].includes(server.status)).length],
    ["Offline", serverRows.filter((server) => server.status === "offline").length],
    ["Active alerts", activeAlerts],
    ["Average CPU", average(serverRows.map((server) => server.cpu_usage_percent))],
    ["Average memory", average(serverRows.map((server) => server.memory_usage_percent))],
    ["Average disk", average(serverRows.map((server) => server.disk_usage_percent))],
  ] as const;
  return (
    <DashboardShell workspace={currentWorkspace} title="Overview" action={<div className="flex gap-2"><Button variant="secondary" disabled={tunnels.isError || tunnelRows.length === 0 || tunnelAction} title={tunnels.isError ? "Run make local-helper first" : `${runningTunnels}/${tunnelRows.length} tunnels connected`} onClick={toggleTunnels}>{allTunnelsRunning ? <Square className="h-4 w-4" /> : <Cable className="h-4 w-4" />}{tunnelAction ? "Working..." : allTunnelsRunning ? `Stop all (${runningTunnels})` : `Start all tunnels (${runningTunnels}/${tunnelRows.length})`}</Button><Button onClick={() => router.push("/servers/add")}><Plus className="h-4 w-4" />Add Server</Button></div>}>
        <div className="mx-auto grid max-w-7xl gap-5 px-5 py-6">
          {tunnelError && <section className="border-l-2 border-danger bg-danger/5 px-4 py-3 text-sm" role="alert">{tunnelError}</section>}
          <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            {summary.map(([label, value]) => (
              <div key={label} className="rounded-lg border border-panel-border bg-panel p-4"><p className="text-sm text-muted">{label}</p><p className="mt-2 text-3xl font-semibold">{value}</p></div>
            ))}
          </section>

          {alerts.isError && <section className="flex items-center justify-between gap-4 border-l-2 border-warning bg-warning/5 px-4 py-3 text-sm"><span>Server data is current, but the alert feed is temporarily unavailable.</span><button className="text-accent hover:underline" onClick={() => alerts.refetch()}>Retry</button></section>}

          {incidents.length > 0 && <section className="overflow-hidden rounded-lg border border-panel-border bg-panel"><div className="flex items-center gap-2 border-b border-panel-border px-4 py-3"><Bell className="h-4 w-4 text-muted" /><h2 className="text-sm font-semibold">Recent incidents</h2></div><div className="divide-y divide-panel-border">{incidents.slice(0, 5).map((event) => <Link key={event.id} href={`/servers/${event.server_id}`} className="grid gap-2 px-4 py-3 text-sm hover:bg-background/40 sm:grid-cols-[minmax(0,1fr)_100px_180px] sm:items-center"><span className="truncate font-medium">{event.rule_name}</span><span className={event.state === "firing" ? "text-danger" : "text-muted"}>{event.state}</span><time className="text-xs text-muted">{new Date(event.triggered_at).toLocaleString()}</time></Link>)}</div></section>}

          {serverRows.length === 0 ? <section id="servers" className="grid min-h-[360px] place-items-center rounded-lg border border-dashed border-panel-border px-6 text-center">
            <div className="max-w-md">
              <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-md border border-panel-border bg-panel"><Server className="h-5 w-5 text-muted" /></span>
              <h2 className="mt-5 text-base font-semibold">No servers enrolled</h2>
              <p className="mt-2 text-sm leading-6 text-muted">Generate a short-lived enrollment token, then connect the local Linux agent binary.</p>
              <div className="mt-5 flex items-center justify-center gap-2 text-xs text-muted"><ShieldCheck className="h-4 w-4 text-accent" />Workspace access is isolated and audited</div>
            </div>
          </section> : <section id="servers">
            <div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">Servers</h2><Link href="/servers" className="flex items-center gap-1 text-xs text-muted hover:text-accent">View all<ArrowUpRight className="h-3.5 w-3.5" /></Link></div>
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">{serverRows.map((server) => <Link key={server.id} href={`/servers/${server.id}`} className="group rounded-lg border border-panel-border bg-panel p-4 transition hover:border-accent/40 hover:bg-panel/80">
              <div className="flex min-w-0 items-start justify-between gap-4">
                <div className="flex min-w-0 gap-3"><span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-panel-border bg-background"><Server className="h-4 w-4 text-muted" /></span><div className="min-w-0"><h3 className="truncate text-sm font-semibold group-hover:text-accent">{server.name}</h3><p className="mt-1 truncate text-xs text-muted">{server.hostname || "Awaiting identity"} · {server.environment}</p></div></div>
                <StatusPill status={server.status} />
              </div>
              <div className="mt-5 grid grid-cols-3 gap-4">
                <ResourceMetric label="CPU" value={server.cpu_usage_percent} />
                <ResourceMetric label="Memory" value={server.memory_usage_percent} />
                <ResourceMetric label="Disk" value={server.disk_usage_percent} />
              </div>
              <div className="mt-5 flex items-center justify-between gap-4 border-t border-panel-border pt-3 text-xs text-muted"><span>Uptime <strong className="ml-1 font-medium text-foreground">{formatUptime(server.uptime_seconds)}</strong></span><span className="truncate">Seen {formatTime(server.last_seen_at)}</span></div>
            </Link>)}</div>
          </section>}
        </div>
    </DashboardShell>
  );
}

function formatPercent(value: number | null) { return value == null ? "—" : `${value.toFixed(1)}%`; }
function ResourceMetric({ label, value }: { label: string; value: number | null }) {
  const width = value == null ? 0 : Math.min(100, Math.max(0, value));
  return <div className="min-w-0"><div className="flex items-baseline justify-between gap-2"><span className="text-xs text-muted">{label}</span><span className="font-mono text-xs">{formatPercent(value)}</span></div><div className="mt-2 h-1.5 overflow-hidden rounded-full bg-background"><span className="block h-full rounded-full bg-accent" style={{ width: `${width}%` }} /></div></div>;
}
function average(values: Array<number | null>) { const available = values.filter((value): value is number => value != null); return available.length ? `${(available.reduce((sum, value) => sum + value, 0) / available.length).toFixed(1)}%` : "—"; }
function formatUptime(value: number | null) {
  if (value == null) return "—";
  const days = Math.floor(value / 86400);
  const hours = Math.floor((value % 86400) / 3600);
  return days > 0 ? `${days}d ${hours}h` : `${hours}h`;
}
function formatTime(value: string | null) { return value ? new Intl.DateTimeFormat(undefined, { dateStyle: "short", timeStyle: "short" }).format(new Date(value)) : "Never"; }

function DashboardSkeleton() {
  return <main className="min-h-screen bg-background p-6 text-foreground"><div className="mx-auto max-w-5xl animate-pulse space-y-5"><div className="h-10 w-44 rounded bg-panel" /><div className="grid gap-3 sm:grid-cols-4">{Array.from({ length: 4 }).map((_, i) => <div key={i} className="h-28 rounded-lg bg-panel" />)}</div><div className="h-96 rounded-lg bg-panel" /></div></main>;
}

function DashboardError({ retry }: { retry: () => void }) {
  return <main className="grid min-h-screen place-items-center bg-background p-6 text-foreground"><div className="max-w-sm text-center"><AlertTriangle className="mx-auto h-7 w-7 text-warning" /><h1 className="mt-4 text-lg font-semibold">Workspace unavailable</h1><p className="mt-2 text-sm text-muted">Check that the API, PostgreSQL, and Redis services are running.</p><Button className="mt-5" variant="secondary" onClick={retry}><RotateCcw className="h-4 w-4" />Retry</Button></div></main>;
}

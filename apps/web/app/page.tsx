"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Activity, AlertTriangle, LogOut, Plus, RotateCcw, Server, ShieldCheck } from "lucide-react";
import { useAuth } from "@/app/providers";
import { type ServerRecord, type Workspace } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { StatusPill } from "@/components/status-pill";

export default function Home() {
  const router = useRouter();
  const auth = useAuth();
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
  const summary = [
    ["Total servers", serverRows.length],
    ["Online", serverRows.filter((server) => server.status === "online").length],
    ["Warning", serverRows.filter((server) => ["warning", "degraded"].includes(server.status)).length],
    ["Offline", serverRows.filter((server) => server.status === "offline").length],
  ] as const;
  return (
    <main className="min-h-screen bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 hidden w-64 border-r border-panel-border bg-[#09131d] p-5 lg:flex lg:flex-col">
        <div className="flex items-center gap-3">
          <span className="flex h-9 w-9 items-center justify-center rounded-md border border-accent/40 bg-accent/10"><Activity className="h-5 w-5 text-accent" /></span>
          <div><p className="text-sm font-semibold">NoxWatch</p><p className="text-xs text-muted">{currentWorkspace.name}</p></div>
        </div>
        <nav className="mt-10 grid gap-1 text-sm text-muted">
          {[
            ["Overview", true], ["Servers", false], ["Alerts", false], ["Integrations", false], ["Team", false], ["Audit Logs", false], ["Settings", false],
          ].map(([item, active]) => <span key={String(item)} className={`rounded-md px-3 py-2 ${active ? "bg-panel text-foreground" : "opacity-60"}`}>{item}</span>)}
        </nav>
        <button className="mt-auto flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm text-muted hover:bg-panel hover:text-foreground" onClick={() => auth.logout().then(() => router.replace("/login"))}>
          <LogOut className="h-4 w-4" /> Sign out
        </button>
      </aside>

      <section className="lg:pl-64">
        <header className="sticky top-0 z-10 border-b border-panel-border bg-background/95 px-5 py-4 backdrop-blur">
          <div className="mx-auto flex max-w-7xl items-center justify-between gap-4">
            <div><h1 className="text-xl font-semibold">Overview</h1><p className="text-sm text-muted">{currentWorkspace.name} · {currentWorkspace.role}</p></div>
            <Button onClick={() => router.push("/servers/add")}><Plus className="h-4 w-4" />Add Server</Button>
          </div>
        </header>

        <div className="mx-auto grid max-w-7xl gap-5 px-5 py-6">
          <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            {summary.map(([label, value]) => (
              <div key={label} className="rounded-lg border border-panel-border bg-panel p-4"><p className="text-sm text-muted">{label}</p><p className="mt-2 text-3xl font-semibold">{value}</p></div>
            ))}
          </section>

          {serverRows.length === 0 ? <section className="grid min-h-[360px] place-items-center rounded-lg border border-dashed border-panel-border px-6 text-center">
            <div className="max-w-md">
              <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-md border border-panel-border bg-panel"><Server className="h-5 w-5 text-muted" /></span>
              <h2 className="mt-5 text-base font-semibold">No servers enrolled</h2>
              <p className="mt-2 text-sm leading-6 text-muted">Generate a short-lived enrollment token, then connect the local Linux agent binary.</p>
              <div className="mt-5 flex items-center justify-center gap-2 text-xs text-muted"><ShieldCheck className="h-4 w-4 text-accent" />Workspace access is isolated and audited</div>
            </div>
          </section> : <section className="overflow-hidden rounded-lg border border-panel-border bg-panel">
            <div className="border-b border-panel-border px-4 py-3"><h2 className="text-sm font-semibold">Servers</h2></div>
            <div className="overflow-x-auto"><table className="w-full min-w-[820px] text-left text-sm">
              <thead className="text-muted"><tr><th className="px-4 py-3 font-medium">Server</th><th className="px-4 py-3 font-medium">Status</th><th className="px-4 py-3 font-medium">CPU</th><th className="px-4 py-3 font-medium">Memory</th><th className="px-4 py-3 font-medium">Disk</th><th className="px-4 py-3 font-medium">Uptime</th><th className="px-4 py-3 font-medium">Last seen</th></tr></thead>
              <tbody>{serverRows.map((server) => <tr key={server.id} className="border-t border-panel-border">
                <td className="px-4 py-4"><Link href={`/servers/${server.id}`} className="font-medium hover:text-accent">{server.name}</Link><p className="mt-1 text-xs text-muted">{server.hostname || "Awaiting identity"} · {server.environment}</p></td>
                <td className="px-4 py-4"><StatusPill status={server.status} /></td>
                <td className="px-4 py-4 font-mono text-xs">{formatPercent(server.cpu_usage_percent)}</td>
                <td className="px-4 py-4 font-mono text-xs">{formatPercent(server.memory_usage_percent)}</td>
                <td className="px-4 py-4 font-mono text-xs">{formatPercent(server.disk_usage_percent)}</td>
                <td className="px-4 py-4 text-muted">{formatUptime(server.uptime_seconds)}</td>
                <td className="px-4 py-4 text-muted">{formatTime(server.last_seen_at)}</td>
              </tr>)}</tbody>
            </table></div>
          </section>}
        </div>
      </section>
    </main>
  );
}

function formatPercent(value: number | null) { return value == null ? "—" : `${value.toFixed(1)}%`; }
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

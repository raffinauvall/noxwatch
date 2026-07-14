"use client";

import { useDeferredValue, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Grid3X3, List, Plus, RotateCcw, Search, Server } from "lucide-react";
import { useAuth } from "@/app/providers";
import { DashboardShell } from "@/components/dashboard-shell";
import { StatusPill } from "@/components/status-pill";
import { Button } from "@/components/ui/button";
import { type ServerRecord, type Workspace } from "@/lib/api";

const pageSize = 24;

export default function ServersPage() {
  const auth = useAuth();
  const router = useRouter();
  const [search, setSearch] = useState("");
  const deferredSearch = useDeferredValue(search);
  const [status, setStatus] = useState("");
  const [environment, setEnvironment] = useState("");
  const [tag, setTag] = useState("");
  const [sort, setSort] = useState("recent");
  const [page, setPage] = useState(0);
  const [view, setView] = useState<"table" | "cards">("table");
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: () => auth.request<Workspace[]>("/api/v1/workspaces"), enabled: Boolean(auth.accessToken) });
  const workspace = workspaces.data?.[0];
  const servers = useQuery({
    queryKey: ["server-inventory", workspace?.id, deferredSearch, status, environment, tag, sort, page],
    queryFn: () => {
      const query = new URLSearchParams({ workspace_id: workspace?.id ?? "", limit: String(pageSize), offset: String(page * pageSize), sort });
      if (deferredSearch) query.set("search", deferredSearch);
      if (status) query.set("status", status);
      if (environment) query.set("environment", environment);
      if (tag) query.set("tag", tag);
      return auth.request<ServerRecord[]>(`/api/v1/servers?${query}`);
    },
    enabled: Boolean(auth.accessToken && workspace?.id),
  });

  useEffect(() => { if (!auth.loading && !auth.user) router.replace("/login"); }, [auth.loading, auth.user, router]);
  if (auth.loading || !auth.user || workspaces.isLoading || !workspace) return <main className="min-h-screen animate-pulse bg-background" />;
  const rows = servers.data ?? [];
  return <DashboardShell workspace={workspace} title="Servers" description="Search and inspect enrolled infrastructure" action={<Button onClick={() => router.push("/servers/add")}><Plus className="h-4 w-4" />Add Server</Button>}>
    <div className="mx-auto grid max-w-7xl gap-5 px-5 py-6">
      <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(220px,1fr)_160px_170px_160px_140px_auto]">
        <label className="relative"><Search className="pointer-events-none absolute left-3 top-3.5 h-4 w-4 text-muted" /><input className="form-control pl-10" value={search} onChange={(event) => { setSearch(event.target.value); setPage(0); }} placeholder="Search name or hostname" /></label>
        <select className="form-control" value={status} onChange={(event) => { setStatus(event.target.value); setPage(0); }} aria-label="Status filter"><option value="">All statuses</option>{["online","warning","degraded","offline","unknown","maintenance"].map((value) => <option key={value} value={value}>{title(value)}</option>)}</select>
        <select className="form-control" value={environment} onChange={(event) => { setEnvironment(event.target.value); setPage(0); }} aria-label="Environment filter"><option value="">All environments</option>{["production","staging","development","testing","other"].map((value) => <option key={value} value={value}>{title(value)}</option>)}</select>
        <input className="form-control" value={tag} onChange={(event) => { setTag(event.target.value); setPage(0); }} placeholder="Tag, e.g. role:api" aria-label="Tag filter" />
        <select className="form-control" value={sort} onChange={(event) => { setSort(event.target.value); setPage(0); }} aria-label="Sort servers"><option value="recent">Recently added</option><option value="name">Name</option><option value="status">Status</option></select>
        <div className="flex h-11 rounded-md border border-panel-border bg-panel p-1"><button title="Table view" className={`w-10 rounded ${view === "table" ? "bg-background text-foreground" : "text-muted"}`} onClick={() => setView("table")}><List className="mx-auto h-4 w-4" /></button><button title="Card view" className={`w-10 rounded ${view === "cards" ? "bg-background text-foreground" : "text-muted"}`} onClick={() => setView("cards")}><Grid3X3 className="mx-auto h-4 w-4" /></button></div>
      </section>

      {servers.isLoading ? <div className="h-96 animate-pulse rounded-lg bg-panel" /> : servers.isError ? <div className="grid h-80 place-items-center rounded-lg border border-panel-border"><Button variant="secondary" onClick={() => servers.refetch()}><RotateCcw className="h-4 w-4" />Retry inventory</Button></div> : rows.length === 0 ? <div className="grid h-80 place-items-center rounded-lg border border-dashed border-panel-border text-center"><div><Server className="mx-auto h-6 w-6 text-muted" /><h2 className="mt-4 text-sm font-semibold">No matching servers</h2><p className="mt-2 text-sm text-muted">Adjust filters or enroll a new Linux server.</p></div></div> : view === "table" ? <ServerTable rows={rows} /> : <ServerCards rows={rows} />}

      <div className="flex items-center justify-between"><p className="text-xs text-muted">Page {page + 1} · up to {pageSize} servers per page</p><div className="flex gap-2"><Button variant="secondary" disabled={page === 0} onClick={() => setPage((value) => value - 1)}><ChevronLeft className="h-4 w-4" />Previous</Button><Button variant="secondary" disabled={rows.length < pageSize} onClick={() => setPage((value) => value + 1)}>Next<ChevronRight className="h-4 w-4" /></Button></div></div>
    </div>
  </DashboardShell>;
}

function ServerTable({ rows }: { rows: ServerRecord[] }) { return <section className="overflow-hidden rounded-lg border border-panel-border bg-panel"><div className="overflow-x-auto"><table className="w-full min-w-[900px] text-left text-sm"><thead className="text-muted"><tr>{["Server","Status","Environment","CPU","Memory","Disk","Last seen"].map((label) => <th key={label} className="px-4 py-3 font-medium">{label}</th>)}</tr></thead><tbody>{rows.map((server) => <tr key={server.id} className="border-t border-panel-border"><td className="px-4 py-4"><Link href={`/servers/${server.id}`} className="font-medium hover:text-accent">{server.name}</Link><p className="mt-1 text-xs text-muted">{server.hostname || "Awaiting identity"}</p></td><td className="px-4 py-4"><StatusPill status={server.status} /></td><td className="px-4 py-4 text-muted">{title(server.environment)}</td><td className="px-4 py-4 font-mono text-xs">{percent(server.cpu_usage_percent)}</td><td className="px-4 py-4 font-mono text-xs">{percent(server.memory_usage_percent)}</td><td className="px-4 py-4 font-mono text-xs">{percent(server.disk_usage_percent)}</td><td className="px-4 py-4 text-xs text-muted">{server.last_seen_at ? new Date(server.last_seen_at).toLocaleString() : "Never"}</td></tr>)}</tbody></table></div></section>; }
function ServerCards({ rows }: { rows: ServerRecord[] }) { return <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">{rows.map((server) => <Link key={server.id} href={`/servers/${server.id}`} className="rounded-lg border border-panel-border bg-panel p-4 hover:border-accent/40"><div className="flex items-start justify-between gap-3"><div className="min-w-0"><h2 className="truncate text-sm font-semibold">{server.name}</h2><p className="mt-1 truncate text-xs text-muted">{server.hostname || "Awaiting identity"} · {title(server.environment)}</p></div><StatusPill status={server.status} /></div><div className="mt-5 grid grid-cols-3 gap-3 text-xs"><Metric label="CPU" value={percent(server.cpu_usage_percent)} /><Metric label="Memory" value={percent(server.memory_usage_percent)} /><Metric label="Disk" value={percent(server.disk_usage_percent)} /></div><div className="mt-4 flex flex-wrap gap-1">{server.tags.slice(0, 4).map((tag) => <span key={tag} className="rounded bg-background px-2 py-1 text-[10px] text-muted">{tag}</span>)}</div></Link>)}</section>; }
function Metric({ label, value }: { label: string; value: string }) { return <div><p className="text-muted">{label}</p><p className="mt-1 font-mono">{value}</p></div>; }
function percent(value: number | null) { return value == null ? "—" : `${value.toFixed(1)}%`; }
function title(value: string) { return value.charAt(0).toUpperCase() + value.slice(1); }

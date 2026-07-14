"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Activity, AlertTriangle, ArrowLeft, Bell, Cpu, Database, HardDrive, MemoryStick, Network, RotateCcw } from "lucide-react";
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { useAuth } from "@/app/providers";
import { type AlertEvent, type MetricSample, type MetricSnapshot, type ServerRecord } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { StatusPill } from "@/components/status-pill";

const ranges = [{ label: "1h", hours: 1 }, { label: "6h", hours: 6 }, { label: "24h", hours: 24 }, { label: "7d", hours: 168 }, { label: "30d", hours: 720 }];

export default function ServerPage() {
  const auth = useAuth();
  const router = useRouter();
  const serverID = useParams<{ serverId: string }>().serverId;
  const [hours, setHours] = useState(24);
  const server = useQuery({ queryKey: ["server", serverID], queryFn: () => auth.request<ServerRecord>(`/api/v1/servers/${serverID}`), enabled: Boolean(auth.accessToken), refetchInterval: 20_000 });
  const latest = useQuery({ queryKey: ["metrics-latest", serverID], queryFn: () => auth.request<MetricSnapshot>(`/api/v1/servers/${serverID}/metrics/latest`), enabled: Boolean(auth.accessToken), retry: false, refetchInterval: 30_000 });
  const history = useQuery({ queryKey: ["metrics", serverID, hours], queryFn: () => { const to = new Date(); const from = new Date(to.getTime() - hours * 3600_000); return auth.request<MetricSample[]>(`/api/v1/servers/${serverID}/metrics?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}&limit=2000`); }, enabled: Boolean(auth.accessToken) });
  const alerts = useQuery({ queryKey: ["server-alerts", serverID], queryFn: () => auth.request<AlertEvent[]>(`/api/v1/servers/${serverID}/alerts`), enabled: Boolean(auth.accessToken), refetchInterval: 30_000 });

  useEffect(() => { if (!auth.loading && !auth.user) router.replace("/login"); }, [auth.loading, auth.user, router]);
  if (auth.loading || !auth.user || server.isLoading) return <DetailSkeleton />;
  if (server.isError || !server.data) return <DetailError retry={() => server.refetch()} />;
  const item = server.data;
  const samples = history.data ?? [];
  const chartData = samples.map((sample) => ({ ...sample, time: new Date(sample.collected_at).getTime() }));

  return <main className="min-h-screen bg-background text-foreground">
    <header className="border-b border-panel-border px-5 py-5"><div className="mx-auto flex max-w-7xl items-center justify-between gap-5"><div><Link href="/" className="mb-3 flex items-center gap-2 text-sm text-muted hover:text-foreground"><ArrowLeft className="h-4 w-4" />Overview</Link><div className="flex flex-wrap items-center gap-3"><h1 className="text-2xl font-semibold">{item.name}</h1><StatusPill status={item.status} /></div><p className="mt-2 text-sm text-muted">{item.hostname} · {item.environment} · {item.operating_system} {item.os_version}</p></div><div className="flex rounded-md border border-panel-border p-1">{ranges.map((range) => <button key={range.hours} className={`h-8 min-w-10 px-2 text-xs ${hours === range.hours ? "rounded bg-panel text-foreground" : "text-muted"}`} onClick={() => setHours(range.hours)}>{range.label}</button>)}</div></div></header>
    <div className="mx-auto grid max-w-7xl gap-5 px-5 py-6">
      {(item.status === "offline" || item.agent_revoked) && <section className="flex gap-3 border-l-2 border-danger bg-danger/5 px-4 py-3"><AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-danger" /><div><h2 className="text-sm font-semibold">Agent disconnected</h2><p className="mt-1 text-sm text-muted">Check the systemd service, API reachability, and credential status. Re-enrollment requires a new one-time token.</p></div></section>}
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={<Cpu />} label="CPU" value={percent(item.cpu_usage_percent)} />
        <MetricCard icon={<MemoryStick />} label="Memory" value={percent(item.memory_usage_percent)} />
        <MetricCard icon={<HardDrive />} label="Peak disk" value={percent(item.disk_usage_percent)} />
        <MetricCard icon={<Activity />} label="Uptime" value={uptime(item.uptime_seconds)} />
      </section>

      {history.isLoading ? <ChartSkeleton /> : history.isError ? <ChartError retry={() => history.refetch()} /> : samples.length === 0 ? <EmptyMetrics /> : <>
        <section className="grid gap-5 xl:grid-cols-2"><Chart title="CPU usage" data={chartData} lines={[{ key: "cpu_usage_percent", label: "CPU", color: "#66e0c2" }]} unit="%" /><Chart title="System load" data={chartData} lines={[{ key: "load_1", label: "1m", color: "#66e0c2" }, { key: "load_5", label: "5m", color: "#f5b84b" }, { key: "load_15", label: "15m", color: "#8ba0b7" }]} /></section>
        <section className="grid gap-5 xl:grid-cols-2"><Chart title="Memory usage" data={chartData} lines={[{ key: "memory_usage_percent", label: "Memory", color: "#66e0c2" }]} unit="%" /><Chart title="Swap usage" data={chartData} lines={[{ key: "swap_usage_percent", label: "Swap", color: "#f5b84b" }]} unit="%" /></section>
      </>}

      <section className="grid gap-5 xl:grid-cols-2">
        <div className="rounded-lg border border-panel-border bg-panel"><div className="flex items-center gap-2 border-b border-panel-border px-4 py-3"><Database className="h-4 w-4 text-muted" /><h2 className="text-sm font-semibold">Filesystems</h2></div><div className="divide-y divide-panel-border">{latest.data?.disks?.length ? latest.data.disks.map((disk) => <div key={disk.mount_point} className="p-4"><div className="flex justify-between text-sm"><span>{disk.mount_point} <span className="text-muted">{disk.filesystem}</span></span><span className="font-mono text-xs">{disk.usage_percent.toFixed(1)}%</span></div><div className="mt-3 h-1.5 overflow-hidden rounded bg-background"><div className="h-full bg-accent" style={{ width: `${Math.min(100, disk.usage_percent)}%` }} /></div><p className="mt-2 text-xs text-muted">{bytes(disk.used_bytes)} of {bytes(disk.total_bytes)}</p></div>) : <p className="p-6 text-sm text-muted">No filesystem samples available.</p>}</div></div>
        <div className="rounded-lg border border-panel-border bg-panel"><div className="flex items-center gap-2 border-b border-panel-border px-4 py-3"><Network className="h-4 w-4 text-muted" /><h2 className="text-sm font-semibold">Network traffic</h2></div><div className="divide-y divide-panel-border">{latest.data?.networks?.length ? latest.data.networks.map((network) => <div key={network.interface} className="grid grid-cols-3 gap-3 p-4 text-sm"><span>{network.interface}</span><span className="text-muted">↓ {rate(network.rx_bytes_per_second)}</span><span className="text-muted">↑ {rate(network.tx_bytes_per_second)}</span></div>) : <p className="p-6 text-sm text-muted">No network samples available.</p>}</div></div>
      </section>

      <section className="grid gap-3 rounded-lg border border-panel-border bg-panel p-4 text-sm sm:grid-cols-2 xl:grid-cols-4"><Info label="Kernel" value={item.kernel_version} /><Info label="Architecture" value={item.architecture} /><Info label="Agent" value={item.agent_version} /><Info label="Last heartbeat" value={item.last_seen_at ? new Date(item.last_seen_at).toLocaleString() : "Never"} /></section>
      <section className="overflow-hidden rounded-lg border border-panel-border bg-panel">
        <div className="flex items-center justify-between border-b border-panel-border px-4 py-3"><div className="flex items-center gap-2"><Bell className="h-4 w-4 text-muted" /><h2 className="text-sm font-semibold">Alert history</h2></div><Link href="/alerts" className="text-xs text-accent hover:underline">Manage rules</Link></div>
        {alerts.isLoading ? <div className="h-24 animate-pulse bg-background/30" /> : alerts.data?.length ? <div className="divide-y divide-panel-border">{alerts.data.slice(0, 20).map((event) => <div key={event.id} className="grid gap-2 px-4 py-3 text-sm sm:grid-cols-[minmax(0,1fr)_100px_180px] sm:items-center"><div><p className="font-medium">{event.rule_name}</p><p className="mt-1 text-xs text-muted">{event.current_value.toFixed(1)} at {event.threshold.toFixed(1)} threshold</p></div><span className={event.severity === "critical" ? "text-danger" : "text-warning"}>{event.state}</span><time className="text-xs text-muted">{new Date(event.triggered_at).toLocaleString()}</time></div>)}</div> : <p className="p-6 text-sm text-muted">No alert events recorded for this server.</p>}
      </section>
    </div>
  </main>;
}

function MetricCard({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) { return <div className="rounded-lg border border-panel-border bg-panel p-4"><div className="flex items-center gap-2 text-sm text-muted"><span className="[&>svg]:h-4 [&>svg]:w-4">{icon}</span>{label}</div><p className="mt-3 text-2xl font-semibold">{value}</p></div>; }
function Chart({ title, data, lines, unit = "" }: { title: string; data: Array<MetricSample & { time: number }>; lines: Array<{ key: keyof MetricSample; label: string; color: string }>; unit?: string }) { return <div className="h-80 rounded-lg border border-panel-border bg-panel p-4"><h2 className="mb-5 text-sm font-semibold">{title}</h2><ResponsiveContainer width="100%" height="85%"><LineChart data={data}><CartesianGrid stroke="#203040" vertical={false} /><XAxis dataKey="time" type="number" domain={["dataMin", "dataMax"]} tickFormatter={(value) => new Date(value).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} stroke="#8ba0b7" fontSize={11} /><YAxis stroke="#8ba0b7" fontSize={11} unit={unit} /><Tooltip labelFormatter={(value) => new Date(Number(value)).toLocaleString()} formatter={(value) => [`${Number(value).toFixed(2)}${unit}`, ""]} contentStyle={{ background: "#0e1822", border: "1px solid #203040", borderRadius: 6 }} />{lines.map((line) => <Line key={String(line.key)} type="linear" dataKey={line.key} name={line.label} stroke={line.color} strokeWidth={2} dot={false} connectNulls={false} />)}</LineChart></ResponsiveContainer></div>; }
function Info({ label, value }: { label: string; value: string }) { return <div><p className="text-xs text-muted">{label}</p><p className="mt-1 truncate">{value || "—"}</p></div>; }
function DetailSkeleton() { return <main className="min-h-screen animate-pulse bg-background p-6"><div className="mx-auto max-w-7xl space-y-5"><div className="h-16 rounded bg-panel" /><div className="grid grid-cols-4 gap-3">{[1,2,3,4].map((item) => <div key={item} className="h-28 rounded bg-panel" />)}</div><div className="h-80 rounded bg-panel" /></div></main>; }
function ChartSkeleton() { return <div className="h-80 animate-pulse rounded-lg bg-panel" />; }
function EmptyMetrics() { return <section className="grid h-80 place-items-center rounded-lg border border-dashed border-panel-border text-center"><div><Activity className="mx-auto h-6 w-6 text-muted" /><h2 className="mt-4 text-sm font-semibold">Waiting for first metrics</h2><p className="mt-2 text-sm text-muted">Heartbeat is registered, but no valid metric sample is available yet.</p></div></section>; }
function DetailError({ retry }: { retry: () => void }) { return <main className="grid min-h-screen place-items-center bg-background text-foreground"><div className="text-center"><AlertTriangle className="mx-auto h-7 w-7 text-danger" /><h1 className="mt-4 text-lg font-semibold">Server unavailable</h1><Button className="mt-5" variant="secondary" onClick={retry}><RotateCcw className="h-4 w-4" />Retry</Button></div></main>; }
function ChartError({ retry }: { retry: () => void }) { return <section className="grid h-80 place-items-center rounded-lg border border-panel-border"><Button variant="secondary" onClick={retry}><RotateCcw className="h-4 w-4" />Retry metrics</Button></section>; }
function percent(value: number | null) { return value == null ? "—" : `${value.toFixed(1)}%`; }
function uptime(value: number | null) { if (value == null) return "—"; const days = Math.floor(value / 86400); const hours = Math.floor((value % 86400) / 3600); return `${days}d ${hours}h`; }
function bytes(value: number) { const units = ["B", "KB", "MB", "GB", "TB"]; let size = value; let index = 0; while (size >= 1024 && index < units.length - 1) { size /= 1024; index++; } return `${size.toFixed(index ? 1 : 0)} ${units[index]}`; }
function rate(value: number) { return `${bytes(value)}/s`; }

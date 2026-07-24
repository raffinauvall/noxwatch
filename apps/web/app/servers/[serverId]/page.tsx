"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Activity, AlertTriangle, ArrowLeft, Ban, Bell, Cable, Cpu, Database, HardDrive, MemoryStick, Network, RotateCcw, Terminal, Trash2, Wrench } from "lucide-react";
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { useAuth } from "@/app/providers";
import { type AlertEvent, type MetricSample, type MetricSnapshot, type ServerRecord } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { StatusPill } from "@/components/status-pill";
import { localHelper, type TunnelStatus } from "@/lib/local-helper";

const ranges = [{ label: "1h", hours: 1 }, { label: "6h", hours: 6 }, { label: "24h", hours: 24 }, { label: "7d", hours: 168 }, { label: "30d", hours: 720 }];

export default function ServerPage() {
  const auth = useAuth();
  const router = useRouter();
  const serverID = useParams<{ serverId: string }>().serverId;
  const [hours, setHours] = useState(24);
  const [actionError, setActionError] = useState("");
  const [sshUser, setSSHUser] = useState("deploy");
  const [sshHost, setSSHHost] = useState("");
  const [sshPort, setSSHPort] = useState("22");
  const [tunnelBusy, setTunnelBusy] = useState(false);
  const server = useQuery({ queryKey: ["server", serverID], queryFn: () => auth.request<ServerRecord>(`/api/v1/servers/${serverID}`), enabled: Boolean(auth.accessToken), refetchInterval: 20_000 });
  const latest = useQuery({ queryKey: ["metrics-latest", serverID], queryFn: () => auth.request<MetricSnapshot>(`/api/v1/servers/${serverID}/metrics/latest`), enabled: Boolean(auth.accessToken), retry: false, refetchInterval: 20_000 });
  const history = useQuery({ queryKey: ["metrics", serverID, hours], queryFn: () => { const to = new Date(); const from = new Date(to.getTime() - hours * 3600_000); return auth.request<MetricSample[]>(`/api/v1/servers/${serverID}/metrics?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}&limit=2000`); }, enabled: Boolean(auth.accessToken), refetchInterval: 20_000 });
  const alerts = useQuery({ queryKey: ["server-alerts", serverID], queryFn: () => auth.request<AlertEvent[]>(`/api/v1/servers/${serverID}/alerts`), enabled: Boolean(auth.accessToken), refetchInterval: 30_000 });
  const tunnels = useQuery({ queryKey: ["local-tunnels"], queryFn: () => localHelper<TunnelStatus[]>("/tunnels"), retry: false, refetchInterval: 3_000 });

  useEffect(() => { if (!auth.loading && !auth.user) router.replace("/login"); }, [auth.loading, auth.user, router]);
  if (auth.loading || !auth.user || server.isLoading) return <DetailSkeleton />;
  if (server.isError || !server.data) return <DetailError retry={() => server.refetch()} />;
  const item = server.data;
  const tunnel = tunnels.data?.find((profile) => profile.server_id === serverID || profile.id === serverID);
  const tunnelConfigured = Boolean(tunnel || (item.ssh_user && item.ssh_host && item.ssh_port && item.tunnel_remote_port));
  const tunnelTarget = tunnel?.target ?? (item.ssh_user && item.ssh_host ? `${item.ssh_user}@${item.ssh_host}` : "");
  const samples = history.data ?? [];
  const chartData = samples.map((sample) => ({ ...sample, time: new Date(sample.collected_at).getTime() }));

  async function setMaintenance(enabled: boolean) {
	setActionError("");
	try { await auth.request(`/api/v1/servers/${serverID}`, { method: "PATCH", body: JSON.stringify({ maintenance: enabled }) }); await server.refetch(); }
	catch (error) { setActionError(error instanceof Error ? error.message : "Server could not be updated."); }
  }

  async function revokeAgent() {
	setActionError("");
	try { await auth.request(`/api/v1/servers/${serverID}/agent`, { method: "DELETE" }); await server.refetch(); }
	catch (error) { setActionError(error instanceof Error ? error.message : "Agent could not be revoked."); }
  }

  async function deleteServer() {
	if (!window.confirm(`Delete ${item.name} and all of its metric history?`)) return;
	setActionError("");
	try { await auth.request(`/api/v1/servers/${serverID}`, { method: "DELETE" }); router.replace("/"); }
	catch (error) { setActionError(error instanceof Error ? error.message : "Server could not be deleted."); }
  }

  async function configureTunnel() {
    if (!/^[A-Za-z0-9._-]+$/.test(sshUser) || !/^[A-Za-z0-9.-]+$/.test(sshHost) || !/^\d{1,5}$/.test(sshPort) || Number(sshPort) < 1 || Number(sshPort) > 65535) {
      setActionError("Enter a valid SSH username, host, and port.");
      return;
    }
    setTunnelBusy(true);
    setActionError("");
    try {
      await auth.request(`/api/v1/servers/${serverID}/tunnel`, { method: "PUT", body: JSON.stringify({ ssh_user: sshUser, ssh_host: sshHost, ssh_port: Number(sshPort), remote_port: 18082 }) });
      await localHelper("/tunnels/register", "POST", { id: serverID, server_id: serverID, name: item.name, target: `${sshUser}@${sshHost}`, port: sshPort, remote_port: "18082" });
      await localHelper("/tunnels/start", "POST", { id: serverID });
      await server.refetch();
      await tunnels.refetch();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "Tunnel could not be configured.");
    } finally {
      setTunnelBusy(false);
    }
  }

  async function toggleTunnel() {
    if (!tunnelConfigured) return;
    const stopping = Boolean(tunnel?.running);
    setTunnelBusy(true);
    setActionError("");
    try {
      if (!tunnel) {
        await localHelper("/tunnels/register", "POST", { id: serverID, server_id: serverID, name: item.name, target: tunnelTarget, port: String(item.ssh_port), remote_port: String(item.tunnel_remote_port) });
      }
      await localHelper(stopping ? "/tunnels/stop" : "/tunnels/start", "POST", { id: tunnel?.id ?? serverID });
      if (stopping) await auth.request(`/api/v1/servers/${serverID}/disconnect`, { method: "POST" });
      await server.refetch();
      await tunnels.refetch();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "Tunnel action failed.");
    } finally {
      setTunnelBusy(false);
    }
  }

  async function openTerminal() {
    if (!tunnelConfigured) return;
    setTunnelBusy(true);
    setActionError("");
    try {
      if (!tunnel) {
        await localHelper("/tunnels/register", "POST", { id: serverID, server_id: serverID, name: item.name, target: tunnelTarget, port: String(item.ssh_port), remote_port: String(item.tunnel_remote_port) });
      }
      await localHelper("/terminal", "POST", { id: tunnel?.id ?? serverID });
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "SSH terminal could not be opened.");
    } finally {
      setTunnelBusy(false);
    }
  }

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
      <section className="rounded-lg border border-panel-border bg-panel p-4">
        <div className="flex flex-wrap items-center justify-between gap-4"><div><div className="flex items-center gap-2"><Cable className="h-4 w-4 text-accent" /><h2 className="text-sm font-semibold">Reverse tunnel</h2></div><p className="mt-1 text-xs text-muted">{tunnels.isError ? "Local helper unavailable; click to retry after it starts." : tunnelConfigured ? `${tunnelTarget}:${tunnel?.port ?? item.ssh_port} · ${tunnel?.running ? "Connected" : "Disconnected"}` : "Configure the SSH target once for this existing server."}</p></div>{tunnelConfigured && <div className="flex gap-2"><Button variant="secondary" disabled={tunnelBusy} onClick={openTerminal}><Terminal className="h-4 w-4" />Open terminal</Button><Button variant="secondary" disabled={tunnelBusy} onClick={toggleTunnel}>{tunnelBusy ? "Working..." : tunnel?.running ? "Stop tunnel" : "Start tunnel"}</Button></div>}</div>
        {!tunnelConfigured && <div className="mt-4 grid gap-3 sm:grid-cols-[160px_minmax(180px,1fr)_110px_auto]"><input className="form-control" value={sshUser} onChange={(event) => setSSHUser(event.target.value)} placeholder="SSH user" aria-label="SSH username" /><input className="form-control" value={sshHost} onChange={(event) => setSSHHost(event.target.value)} placeholder="Server IP or hostname" aria-label="SSH host" /><input className="form-control" type="number" min="1" max="65535" value={sshPort} onChange={(event) => setSSHPort(event.target.value)} aria-label="SSH port" /><Button disabled={tunnelBusy} onClick={configureTunnel}>{tunnelBusy ? "Opening..." : "Save & start"}</Button></div>}
      </section>
      <section className="flex flex-wrap items-center justify-between gap-4 border-t border-panel-border py-4"><div><h2 className="text-sm font-semibold">Server controls</h2><p className="mt-1 text-xs text-muted">Sensitive changes are workspace-scoped and recorded in the audit log.</p>{actionError && <p className="mt-2 text-xs text-danger" role="alert">{actionError}</p>}</div><div className="flex flex-wrap gap-2"><Button variant="secondary" onClick={() => setMaintenance(item.status !== "maintenance")}><Wrench className="h-4 w-4" />{item.status === "maintenance" ? "End maintenance" : "Maintenance"}</Button><Button variant="secondary" disabled={item.agent_revoked} onClick={revokeAgent}><Ban className="h-4 w-4" />Revoke agent</Button><Button variant="secondary" className="text-danger" onClick={deleteServer}><Trash2 className="h-4 w-4" />Delete server</Button></div></section>
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

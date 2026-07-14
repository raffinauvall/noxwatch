"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useForm, useWatch } from "react-hook-form";
import { z } from "zod";
import { AlertTriangle, BellPlus, LoaderCircle, Power, Trash2 } from "lucide-react";
import { useAuth } from "@/app/providers";
import { DashboardShell } from "@/components/dashboard-shell";
import { Button } from "@/components/ui/button";
import { type AlertRule, type ServerRecord, type Workspace } from "@/lib/api";

const schema = z.object({
  name: z.string().trim().min(1, "Rule name is required.").max(120),
  server_id: z.string().min(1, "Select a server."),
  metric: z.enum(["cpu_usage", "memory_usage", "disk_usage", "swap_usage", "server_offline", "agent_disconnected"]),
  warning_threshold: z.coerce.number().min(0).max(100),
  critical_threshold: z.coerce.number().min(0).max(100),
  evaluation_seconds: z.coerce.number().int().min(0).max(86400),
  cooldown_seconds: z.coerce.number().int().min(0).max(604800),
});
type Values = z.infer<typeof schema>;
const metricNames: Record<Values["metric"], string> = { cpu_usage: "CPU usage", memory_usage: "Memory usage", disk_usage: "Disk usage", swap_usage: "Swap usage", server_offline: "Server offline", agent_disconnected: "Agent disconnected" };

export default function AlertsPage() {
  const auth = useAuth();
  const router = useRouter();
  const [requestError, setRequestError] = useState("");
  const [saving, setSaving] = useState(false);
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: () => auth.request<Workspace[]>("/api/v1/workspaces"), enabled: Boolean(auth.accessToken) });
  const workspace = workspaces.data?.[0];
  const servers = useQuery({ queryKey: ["servers", workspace?.id], queryFn: () => auth.request<ServerRecord[]>(`/api/v1/servers?workspace_id=${workspace?.id}`), enabled: Boolean(auth.accessToken && workspace?.id) });
  const rules = useQuery({ queryKey: ["alert-rules", workspace?.id], queryFn: () => auth.request<AlertRule[]>(`/api/v1/alert-rules?workspace_id=${workspace?.id}`), enabled: Boolean(auth.accessToken && workspace?.id) });
  const { register, handleSubmit, control, reset, setError, formState: { errors } } = useForm<Values>({ defaultValues: { metric: "cpu_usage", warning_threshold: 80, critical_threshold: 90, evaluation_seconds: 300, cooldown_seconds: 900 } });
  const metric = useWatch({ control, name: "metric" });
  const connectivity = metric === "server_offline" || metric === "agent_disconnected";

  useEffect(() => { if (!auth.loading && !auth.user) router.replace("/login"); }, [auth.loading, auth.user, router]);

  const create = handleSubmit(async (values) => {
    if (!workspace) return;
    const parsed = schema.safeParse(values);
    if (!parsed.success) {
      for (const issue of parsed.error.issues) setError(issue.path[0] as keyof Values, { message: issue.message });
      return;
    }
    if (parsed.data.critical_threshold < parsed.data.warning_threshold) {
      setError("critical_threshold", { message: "Critical must be at least warning." });
      return;
    }
    setSaving(true);
    setRequestError("");
    try {
      await auth.request("/api/v1/alert-rules", { method: "POST", body: JSON.stringify({ ...parsed.data, workspace_id: workspace.id, warning_threshold: connectivity ? null : parsed.data.warning_threshold, critical_threshold: connectivity ? null : parsed.data.critical_threshold }) });
      reset({ metric: "cpu_usage", warning_threshold: 80, critical_threshold: 90, evaluation_seconds: 300, cooldown_seconds: 900, name: "", server_id: parsed.data.server_id });
      await rules.refetch();
    } catch (error) {
      setRequestError(error instanceof Error ? error.message : "Alert rule could not be created.");
    } finally { setSaving(false); }
  });

  async function updateRule(rule: AlertRule, body: object) {
    setRequestError("");
    try {
      await auth.request(`/api/v1/alert-rules/${rule.id}`, { method: "PATCH", body: JSON.stringify(body) });
      await rules.refetch();
    } catch (error) { setRequestError(error instanceof Error ? error.message : "Alert rule could not be updated."); }
  }

  async function deleteRule(rule: AlertRule) {
    setRequestError("");
    try {
      await auth.request(`/api/v1/alert-rules/${rule.id}`, { method: "DELETE" });
      await rules.refetch();
    } catch (error) { setRequestError(error instanceof Error ? error.message : "Alert rule could not be deleted."); }
  }

  if (auth.loading || !auth.user || workspaces.isLoading || !workspace) return <PageLoading />;
  return <DashboardShell workspace={workspace} title="Alerts" description="Threshold and connectivity rules">
    <div className="mx-auto grid max-w-7xl gap-6 px-5 py-6 xl:grid-cols-[minmax(0,1fr)_380px]">
      <section className="min-w-0">
        <div className="mb-4 flex items-center justify-between"><div><h2 className="text-sm font-semibold">Alert rules</h2><p className="mt-1 text-xs text-muted">Duration prevents a single noisy sample from firing.</p></div><span className="text-xs text-muted">{rules.data?.length ?? 0} configured</span></div>
        {rules.isLoading ? <ListSkeleton /> : rules.isError ? <ErrorState retry={() => rules.refetch()} /> : rules.data?.length ? <div className="grid gap-3">
          {rules.data.map((rule) => {
            const server = servers.data?.find((item) => item.id === rule.server_id);
            return <article key={rule.id} className="grid gap-4 rounded-lg border border-panel-border bg-panel p-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
              <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><h3 className="truncate text-sm font-semibold">{rule.name}</h3><span className={`rounded px-2 py-0.5 text-[11px] ${rule.enabled ? "bg-accent/10 text-accent" : "bg-background text-muted"}`}>{rule.enabled ? "Enabled" : "Disabled"}</span></div><p className="mt-2 text-xs text-muted"><Link className="hover:text-accent" href={`/servers/${rule.server_id}`}>{server?.name ?? "Unavailable server"}</Link> · {describeRule(rule)}</p></div>
              <div className="flex gap-2"><Button title={rule.enabled ? "Disable rule" : "Enable rule"} variant="secondary" className="w-10 px-0" onClick={() => updateRule(rule, { enabled: !rule.enabled })}><Power className="h-4 w-4" /></Button><Button title="Delete rule" variant="secondary" className="w-10 px-0 text-danger" onClick={() => deleteRule(rule)}><Trash2 className="h-4 w-4" /></Button></div>
            </article>;
          })}
        </div> : <div className="grid min-h-72 place-items-center rounded-lg border border-dashed border-panel-border text-center"><div><AlertTriangle className="mx-auto h-6 w-6 text-muted" /><h3 className="mt-4 text-sm font-semibold">No alert rules</h3><p className="mt-2 text-sm text-muted">Create the first rule from the configuration panel.</p></div></div>}
      </section>

      <form className="h-fit rounded-lg border border-panel-border bg-panel p-5" onSubmit={create}>
        <div className="flex items-center gap-2"><BellPlus className="h-4 w-4 text-accent" /><h2 className="text-sm font-semibold">New alert rule</h2></div>
        <div className="mt-5 grid gap-4">
          <Field label="Rule name" error={errors.name?.message}><input className="form-control" {...register("name")} placeholder="High CPU" /></Field>
          <Field label="Server" error={errors.server_id?.message}><select className="form-control" {...register("server_id")}><option value="">Select server</option>{servers.data?.map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}</select></Field>
          <Field label="Metric" error={errors.metric?.message}><select className="form-control" {...register("metric")}>{Object.entries(metricNames).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></Field>
          {!connectivity && <div className="grid grid-cols-2 gap-3"><Field label="Warning %" error={errors.warning_threshold?.message}><input type="number" className="form-control" {...register("warning_threshold")} /></Field><Field label="Critical %" error={errors.critical_threshold?.message}><input type="number" className="form-control" {...register("critical_threshold")} /></Field></div>}
          <div className="grid grid-cols-2 gap-3"><Field label="Duration (s)" error={errors.evaluation_seconds?.message}><input type="number" className="form-control" {...register("evaluation_seconds")} /></Field><Field label="Cooldown (s)" error={errors.cooldown_seconds?.message}><input type="number" className="form-control" {...register("cooldown_seconds")} /></Field></div>
        </div>
        {requestError && <p className="mt-4 text-sm text-danger" role="alert">{requestError}</p>}
        <Button className="mt-5 w-full" type="submit" disabled={saving || !servers.data?.length}>{saving ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <BellPlus className="h-4 w-4" />}Create rule</Button>
      </form>
    </div>
  </DashboardShell>;
}

function describeRule(rule: AlertRule) { const metric = metricNames[rule.metric]; return rule.metric.includes("offline") || rule.metric.includes("disconnected") ? `${metric} for ${rule.evaluation_seconds}s` : `${metric} > ${rule.warning_threshold}% warning / ${rule.critical_threshold}% critical for ${rule.evaluation_seconds}s`; }
function Field({ label, error, children }: { label: string; error?: string; children: React.ReactNode }) { return <label className="grid gap-2 text-xs font-medium"><span>{label}</span>{children}{error && <span className="font-normal text-danger">{error}</span>}</label>; }
function PageLoading() { return <main className="min-h-screen animate-pulse bg-background p-6"><div className="mx-auto h-96 max-w-6xl rounded-lg bg-panel" /></main>; }
function ListSkeleton() { return <div className="grid animate-pulse gap-3">{[1,2,3].map((item) => <div key={item} className="h-24 rounded-lg bg-panel" />)}</div>; }
function ErrorState({ retry }: { retry: () => void }) { return <div className="grid min-h-72 place-items-center rounded-lg border border-panel-border"><Button variant="secondary" onClick={retry}>Retry alert rules</Button></div>; }

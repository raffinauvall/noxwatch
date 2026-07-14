"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Clipboard, Check, LoaderCircle, Plug, Trash2, Webhook } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { useAuth } from "@/app/providers";
import { DashboardShell } from "@/components/dashboard-shell";
import { Button } from "@/components/ui/button";
import { type NotificationChannel, type Workspace } from "@/lib/api";

const schema = z.object({ name: z.string().trim().min(1, "Name is required.").max(100), url: z.string().url("Enter a valid URL.") });
type Values = z.infer<typeof schema>;

export default function IntegrationsPage() {
  const auth = useAuth();
  const router = useRouter();
  const [secret, setSecret] = useState("");
  const [copied, setCopied] = useState(false);
  const [requestError, setRequestError] = useState("");
  const [saving, setSaving] = useState(false);
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: () => auth.request<Workspace[]>("/api/v1/workspaces"), enabled: Boolean(auth.accessToken) });
  const workspace = workspaces.data?.[0];
  const channels = useQuery({ queryKey: ["notification-channels", workspace?.id], queryFn: () => auth.request<NotificationChannel[]>(`/api/v1/notification-channels?workspace_id=${workspace?.id}`), enabled: Boolean(auth.accessToken && workspace?.id) });
  const { register, handleSubmit, reset, setError, formState: { errors } } = useForm<Values>();

  useEffect(() => { if (!auth.loading && !auth.user) router.replace("/login"); }, [auth.loading, auth.user, router]);

  const create = handleSubmit(async (values) => {
    if (!workspace) return;
    const parsed = schema.safeParse(values);
    if (!parsed.success) {
      for (const issue of parsed.error.issues) setError(issue.path[0] as keyof Values, { message: issue.message });
      return;
    }
    setSaving(true); setRequestError(""); setSecret("");
    try {
      const channel = await auth.request<NotificationChannel>("/api/v1/notification-channels", { method: "POST", body: JSON.stringify({ ...parsed.data, workspace_id: workspace.id }) });
      setSecret(channel.secret ?? ""); reset(); await channels.refetch();
    } catch (error) { setRequestError(error instanceof Error ? error.message : "Webhook could not be created."); }
    finally { setSaving(false); }
  });

  async function remove(id: string) {
    setRequestError("");
    try { await auth.request(`/api/v1/notification-channels/${id}`, { method: "DELETE" }); await channels.refetch(); }
    catch (error) { setRequestError(error instanceof Error ? error.message : "Webhook could not be deleted."); }
  }

  if (auth.loading || !auth.user || workspaces.isLoading || !workspace) return <main className="min-h-screen animate-pulse bg-background" />;
  return <DashboardShell workspace={workspace} title="Integrations" description="Signed outbound notifications">
    <div className="mx-auto grid max-w-7xl gap-6 px-5 py-6 xl:grid-cols-[minmax(0,1fr)_380px]">
      <section>
        <div className="mb-4"><h2 className="text-sm font-semibold">Notification channels</h2><p className="mt-1 text-xs text-muted">Webhook delivery is active. Telegram, email, Discord, and Slack are planned.</p></div>
        {channels.isLoading ? <div className="h-28 animate-pulse rounded-lg bg-panel" /> : channels.data?.length ? <div className="grid gap-3">{channels.data.map((channel) => <article key={channel.id} className="flex items-center gap-4 rounded-lg border border-panel-border bg-panel p-4"><span className="flex h-10 w-10 items-center justify-center rounded-md bg-accent/10"><Webhook className="h-5 w-5 text-accent" /></span><div className="min-w-0 flex-1"><h3 className="truncate text-sm font-semibold">{channel.name}</h3><p className="mt-1 text-xs text-muted">Webhook · {channel.enabled ? "Enabled" : "Disabled"} · Added {new Date(channel.created_at).toLocaleDateString()}</p></div><Button title="Delete webhook" variant="secondary" className="w-10 px-0 text-danger" onClick={() => remove(channel.id)}><Trash2 className="h-4 w-4" /></Button></article>)}</div> : <div className="grid min-h-72 place-items-center rounded-lg border border-dashed border-panel-border text-center"><div><Plug className="mx-auto h-6 w-6 text-muted" /><h3 className="mt-4 text-sm font-semibold">No notification channels</h3><p className="mt-2 text-sm text-muted">Add a webhook to receive firing and resolution events.</p></div></div>}
        {secret && <div className="mt-5 border-l-2 border-warning bg-warning/5 p-4"><h3 className="text-sm font-semibold">Signing secret shown once</h3><p className="mt-1 text-xs text-muted">Store this secret now and verify the <code>X-NoxWatch-Signature</code> HMAC-SHA256 header.</p><div className="mt-3 flex gap-2"><code className="min-w-0 flex-1 overflow-x-auto rounded bg-background p-3 text-xs">{secret}</code><Button title="Copy signing secret" variant="secondary" className="w-10 px-0" onClick={async () => { await navigator.clipboard.writeText(secret); setCopied(true); setTimeout(() => setCopied(false), 1500); }}>{copied ? <Check className="h-4 w-4" /> : <Clipboard className="h-4 w-4" />}</Button></div></div>}
      </section>

      <form className="h-fit rounded-lg border border-panel-border bg-panel p-5" onSubmit={create}>
        <div className="flex items-center gap-2"><Webhook className="h-4 w-4 text-accent" /><h2 className="text-sm font-semibold">Add webhook</h2></div>
        <div className="mt-5 grid gap-4"><Field label="Name" error={errors.name?.message}><input className="form-control" {...register("name")} placeholder="Operations webhook" /></Field><Field label="HTTPS endpoint" error={errors.url?.message}><input className="form-control" type="url" {...register("url")} placeholder="https://hooks.example.com/noxwatch" /></Field></div>
        <p className="mt-4 text-xs leading-5 text-muted">Production blocks private network destinations and unsafe redirects. URLs are encrypted at rest.</p>
        {requestError && <p className="mt-4 text-sm text-danger" role="alert">{requestError}</p>}
        <Button className="mt-5 w-full" type="submit" disabled={saving}>{saving ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Webhook className="h-4 w-4" />}Add webhook</Button>
      </form>
    </div>
  </DashboardShell>;
}

function Field({ label, error, children }: { label: string; error?: string; children: React.ReactNode }) { return <label className="grid gap-2 text-xs font-medium"><span>{label}</span>{children}{error && <span className="font-normal text-danger">{error}</span>}</label>; }

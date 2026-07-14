"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Check, ChevronLeft, ChevronRight, CircleDashed, Clipboard, FlaskConical, MonitorUp, Package, RefreshCw, Terminal } from "lucide-react";
import { useAuth } from "@/app/providers";
import { type Workspace } from "@/lib/api";
import { Button } from "@/components/ui/button";

const schema = z.object({
  name: z.string().trim().min(1, "Server name is required.").max(100),
  environment: z.enum(["production", "staging", "development", "testing", "other"]),
  tags: z.string().max(300),
  description: z.string().max(500),
});
type Values = z.infer<typeof schema>;
type Enrollment = { id: string; token?: string; expires_at: string; status: "pending" | "connected" | "expired" | "revoked"; server_id?: string };

export default function AddServerPage() {
  const auth = useAuth();
  const router = useRouter();
  const [step, setStep] = useState(1);
  const [details, setDetails] = useState<Values | null>(null);
  const [enrollment, setEnrollment] = useState<Enrollment | null>(null);
  const [requestError, setRequestError] = useState("");
  const [copied, setCopied] = useState(false);
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: () => auth.request<Workspace[]>("/api/v1/workspaces"), enabled: Boolean(auth.accessToken) });
  const { register, handleSubmit, setError, formState: { errors } } = useForm<Values>({ defaultValues: { environment: "production", tags: "", description: "" } });
  const workspace = workspaces.data?.[0];

  const status = useQuery({
    queryKey: ["enrollment", enrollment?.id],
    queryFn: () => auth.request<Enrollment>(`/api/v1/enrollment-tokens/${enrollment?.id}`),
    enabled: Boolean(enrollment?.id && [3, 4].includes(step)),
    refetchInterval: 3_000,
  });

  useEffect(() => {
    if (!auth.loading && !auth.user) router.replace("/login");
  }, [auth.loading, auth.user, router]);

  const activeStep = status.data?.status === "connected" ? 5 : step;
  const activeEnrollment = status.data?.status === "connected" ? status.data : enrollment;

  const submitInfo = handleSubmit((values) => {
    const parsed = schema.safeParse(values);
    if (!parsed.success) {
      for (const issue of parsed.error.issues) setError(issue.path[0] as keyof Values, { message: issue.message });
      return;
    }
    setDetails(parsed.data);
    setStep(2);
  });

  async function createEnrollment() {
    if (!workspace || !details) return;
    setRequestError("");
    try {
      const next = await auth.request<Enrollment>("/api/v1/servers/enrollment-token", { method: "POST", body: JSON.stringify({
        workspace_id: workspace.id, name: details.name, environment: details.environment, description: details.description,
        tags: details.tags.split(",").map((tag) => tag.trim()).filter(Boolean),
      }) });
      setEnrollment(next);
      setStep(3);
    } catch (error) {
      setRequestError(error instanceof Error ? error.message : "Enrollment token creation failed.");
    }
  }

  async function revoke() {
    if (enrollment?.id && enrollment.status === "pending") await auth.request(`/api/v1/enrollment-tokens/${enrollment.id}`, { method: "DELETE" }).catch(() => undefined);
  }

  async function regenerate() {
    await revoke();
    await createEnrollment();
  }

  const command = (() => {
    if (!enrollment?.token) return "";
    const endpoint = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
    const config = [`endpoint: ${endpoint}`, "server_name: enrolled-server", `environment: ${details?.environment ?? "other"}`, "enrollment_file: /etc/noxwatch/enrollment-token", "credential_file: /etc/noxwatch/credential.json", `allow_insecure_http: ${endpoint.startsWith("http://")}`].join("\\n");
    return `sudo install -d -m 0700 /etc/noxwatch && printf '%s' '${shellQuote(enrollment.token)}' | sudo tee /etc/noxwatch/enrollment-token >/dev/null && printf '%b' '${shellQuote(config)}' | sudo tee /etc/noxwatch/agent.yaml >/dev/null && sudo chmod 0600 /etc/noxwatch/enrollment-token /etc/noxwatch/agent.yaml && sudo noxwatch-agent -config /etc/noxwatch/agent.yaml run`;
  })();

  if (auth.loading || !auth.user || workspaces.isLoading) return <main className="min-h-screen bg-background" />;
  return <main className="min-h-screen bg-background px-5 py-8 text-foreground">
    <div className="mx-auto max-w-3xl">
      <button className="mb-8 flex items-center gap-2 text-sm text-muted hover:text-foreground" onClick={() => { void revoke(); router.push("/"); }}><ChevronLeft className="h-4 w-4" />Back to overview</button>
      <div className="mb-8 flex items-center justify-between gap-3">
        <div><p className="font-mono text-xs uppercase text-accent">Add server</p><h1 className="mt-2 text-2xl font-semibold">Secure agent enrollment</h1></div>
        <p className="text-sm text-muted">Step {activeStep} of 5</p>
      </div>
      <div className="mb-10 grid grid-cols-5 gap-2" aria-hidden="true">{[1, 2, 3, 4, 5].map((item) => <span key={item} className={`h-1 rounded ${item <= activeStep ? "bg-accent" : "bg-panel"}`} />)}</div>

      {activeStep === 1 && <form className="grid gap-5" onSubmit={submitInfo}>
        <Field label="Server name" error={errors.name?.message}><input className="form-control" {...register("name")} autoFocus placeholder="prod-api-01" /></Field>
        <Field label="Environment" error={errors.environment?.message}><select className="form-control" {...register("environment")}><option value="production">Production</option><option value="staging">Staging</option><option value="development">Development</option><option value="testing">Testing</option><option value="other">Other</option></select></Field>
        <Field label="Tags" hint="Comma separated" error={errors.tags?.message}><input className="form-control" {...register("tags")} placeholder="region:sg, role:api" /></Field>
        <Field label="Description" error={errors.description?.message}><textarea className="form-control min-h-24 py-3" {...register("description")} placeholder="Primary API node" /></Field>
        <Button className="mt-3 justify-self-end" type="submit">Continue<ChevronRight className="h-4 w-4" /></Button>
      </form>}

      {activeStep === 2 && <section>
        <h2 className="text-base font-semibold">Installation method</h2>
        <div className="mt-5 grid gap-3">
          <Method icon={<Package />} title="Linux installation script" label="Planned" disabled detail="Enabled after signed release binaries are published." />
          <Method icon={<Terminal />} title="Manual binary" label="Available" detail="Use the locally built static agent and systemd unit." selected />
          <Method icon={<FlaskConical />} title="Docker agent" label="Experimental" disabled detail="Container host metrics are not supported yet." />
        </div>
        {requestError && <p className="mt-4 text-sm text-danger" role="alert">{requestError}</p>}
        <div className="mt-8 flex justify-between"><Button variant="secondary" onClick={() => setStep(1)}><ChevronLeft className="h-4 w-4" />Back</Button><Button onClick={createEnrollment}>Generate token<ChevronRight className="h-4 w-4" /></Button></div>
      </section>}

      {activeStep === 3 && enrollment && <section>
        <div className="flex items-start justify-between gap-4"><div><h2 className="text-base font-semibold">Enrollment command</h2><p className="mt-2 text-sm text-muted">Token expires in <Countdown expiresAt={enrollment.expires_at} /> and is invalidated after first use.</p></div><MonitorUp className="h-5 w-5 text-accent" /></div>
        <pre className="mt-6 overflow-x-auto rounded-md border border-panel-border bg-[#050c12] p-4 text-xs leading-6 text-[#b9cad9]"><code>{command}</code></pre>
        <div className="mt-4 flex flex-wrap gap-3"><Button variant="secondary" onClick={async () => { await navigator.clipboard.writeText(command); setCopied(true); setTimeout(() => setCopied(false), 1500); }}>{copied ? <Check className="h-4 w-4" /> : <Clipboard className="h-4 w-4" />}{copied ? "Copied" : "Copy command"}</Button><Button variant="secondary" onClick={regenerate}><RefreshCw className="h-4 w-4" />Regenerate</Button></div>
        <div className="mt-8 border-t border-panel-border pt-6 text-sm text-muted"><p>Supported: systemd-based x86_64 and arm64 Linux hosts. The agent must already be installed at <code className="text-foreground">/usr/local/bin/noxwatch-agent</code>.</p><p className="mt-2">The command writes only the short-lived token. Permanent credentials are returned directly to the agent and saved with mode 0600.</p></div>
        <div className="mt-8 flex justify-between"><Button variant="secondary" onClick={() => { void revoke(); router.push("/"); }}>Cancel</Button><Button onClick={() => setStep(4)}>I started the agent<ChevronRight className="h-4 w-4" /></Button></div>
      </section>}

      {activeStep === 4 && <section className="grid min-h-72 place-items-center text-center"><div><CircleDashed className="mx-auto h-9 w-9 animate-spin text-accent" /><h2 className="mt-5 text-lg font-semibold">Waiting for connection</h2><p className="mt-2 text-sm text-muted">Last checked {status.dataUpdatedAt ? new Intl.DateTimeFormat(undefined, { timeStyle: "medium" }).format(new Date(status.dataUpdatedAt)) : "Not checked yet"}</p><p className="mt-6 max-w-md text-sm leading-6 text-muted">Check <code className="text-foreground">journalctl -u noxwatch-agent</code>, verify the API endpoint, and confirm the token has not expired.</p><Button className="mt-7" variant="secondary" onClick={() => { void revoke(); router.push("/"); }}>Cancel enrollment</Button></div></section>}

      {activeStep === 5 && activeEnrollment?.server_id && <section className="grid min-h-72 place-items-center text-center"><div><span className="mx-auto flex h-12 w-12 items-center justify-center rounded-md bg-accent/10"><Check className="h-6 w-6 text-accent" /></span><h2 className="mt-5 text-xl font-semibold">Server connected</h2><p className="mt-2 text-sm text-muted">Agent identity is active. The enrollment token is no longer available.</p><Button className="mt-7" onClick={() => router.replace(`/servers/${activeEnrollment.server_id}`)}>Open server dashboard<ChevronRight className="h-4 w-4" /></Button></div></section>}
    </div>
  </main>;
}

function Field({ label, hint, error, children }: { label: string; hint?: string; error?: string; children: React.ReactNode }) {
  return <label className="grid gap-2 text-sm font-medium"><span>{label}{hint && <span className="ml-2 font-normal text-muted">{hint}</span>}</span><span className="contents">{children}</span>{error && <span className="text-xs font-normal text-danger">{error}</span>}</label>;
}
function Method({ icon, title, label, detail, selected, disabled }: { icon: React.ReactNode; title: string; label: string; detail: string; selected?: boolean; disabled?: boolean }) {
  return <div className={`flex items-center gap-4 rounded-lg border p-4 ${selected ? "border-accent/50 bg-accent/5" : "border-panel-border bg-panel opacity-60"}`} aria-disabled={disabled}><span className="text-muted [&>svg]:h-5 [&>svg]:w-5">{icon}</span><div className="min-w-0 flex-1"><div className="flex items-center gap-3"><h3 className="text-sm font-semibold">{title}</h3><span className="text-xs text-muted">{label}</span></div><p className="mt-1 text-xs text-muted">{detail}</p></div>{selected && <Check className="h-4 w-4 text-accent" />}</div>;
}
function Countdown({ expiresAt }: { expiresAt: string }) {
  const [seconds, setSeconds] = useState(15 * 60);
  useEffect(() => { const timer = setInterval(() => setSeconds(Math.max(0, Math.floor((new Date(expiresAt).getTime() - Date.now()) / 1000))), 1000); return () => clearInterval(timer); }, [expiresAt]);
  return <span className="font-mono text-foreground">{Math.floor(seconds / 60)}:{String(seconds % 60).padStart(2, "0")}</span>;
}
function shellQuote(value: string) { return value.replaceAll("'", "'\\''"); }

"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Activity, AlertTriangle, LogOut, Plus, RotateCcw, Server, ShieldCheck } from "lucide-react";
import { useAuth } from "@/app/providers";
import { api, type Workspace } from "@/lib/api";
import { Button } from "@/components/ui/button";

export default function Home() {
  const router = useRouter();
  const auth = useAuth();
  const workspaces = useQuery({
    queryKey: ["workspaces"],
    queryFn: () => api<Workspace[]>("/api/v1/workspaces", {}, auth.accessToken ?? undefined),
    enabled: Boolean(auth.accessToken),
  });

  useEffect(() => {
    if (!auth.loading && !auth.user) router.replace("/login");
  }, [auth.loading, auth.user, router]);

  useEffect(() => {
    if (workspaces.isSuccess && workspaces.data.length === 0) router.replace("/onboarding");
  }, [router, workspaces.data, workspaces.isSuccess]);

  if (auth.loading || !auth.user || workspaces.isLoading) return <DashboardSkeleton />;
  if (workspaces.isError) return <DashboardError retry={() => workspaces.refetch()} />;
  if (!workspaces.data?.length) return <DashboardSkeleton />;

  const workspace = workspaces.data[0];
  return (
    <main className="min-h-screen bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 hidden w-64 border-r border-panel-border bg-[#09131d] p-5 lg:flex lg:flex-col">
        <div className="flex items-center gap-3">
          <span className="flex h-9 w-9 items-center justify-center rounded-md border border-accent/40 bg-accent/10"><Activity className="h-5 w-5 text-accent" /></span>
          <div><p className="text-sm font-semibold">NoxWatch</p><p className="text-xs text-muted">{workspace.name}</p></div>
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
            <div><h1 className="text-xl font-semibold">Overview</h1><p className="text-sm text-muted">{workspace.name} · {workspace.role}</p></div>
            <Button disabled title="Server enrollment is the next implementation phase"><Plus className="h-4 w-4" />Add Server</Button>
          </div>
        </header>

        <div className="mx-auto grid max-w-7xl gap-5 px-5 py-6">
          <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            {[["Total servers", "0"], ["Online", "0"], ["Warning", "0"], ["Active alerts", "0"]].map(([label, value]) => (
              <div key={label} className="rounded-lg border border-panel-border bg-panel p-4"><p className="text-sm text-muted">{label}</p><p className="mt-2 text-3xl font-semibold">{value}</p></div>
            ))}
          </section>

          <section className="grid min-h-[360px] place-items-center rounded-lg border border-dashed border-panel-border px-6 text-center">
            <div className="max-w-md">
              <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-md border border-panel-border bg-panel"><Server className="h-5 w-5 text-muted" /></span>
              <h2 className="mt-5 text-base font-semibold">No servers enrolled</h2>
              <p className="mt-2 text-sm leading-6 text-muted">This workspace is ready. Secure one-command enrollment is the next product phase and is not enabled yet.</p>
              <div className="mt-5 flex items-center justify-center gap-2 text-xs text-muted"><ShieldCheck className="h-4 w-4 text-accent" />Workspace access is isolated and audited</div>
            </div>
          </section>
        </div>
      </section>
    </main>
  );
}

function DashboardSkeleton() {
  return <main className="min-h-screen bg-background p-6 text-foreground"><div className="mx-auto max-w-5xl animate-pulse space-y-5"><div className="h-10 w-44 rounded bg-panel" /><div className="grid gap-3 sm:grid-cols-4">{Array.from({ length: 4 }).map((_, i) => <div key={i} className="h-28 rounded-lg bg-panel" />)}</div><div className="h-96 rounded-lg bg-panel" /></div></main>;
}

function DashboardError({ retry }: { retry: () => void }) {
  return <main className="grid min-h-screen place-items-center bg-background p-6 text-foreground"><div className="max-w-sm text-center"><AlertTriangle className="mx-auto h-7 w-7 text-warning" /><h1 className="mt-4 text-lg font-semibold">Workspace unavailable</h1><p className="mt-2 text-sm text-muted">Check that the API, PostgreSQL, and Redis services are running.</p><Button className="mt-5" variant="secondary" onClick={retry}><RotateCcw className="h-4 w-4" />Retry</Button></div></main>;
}

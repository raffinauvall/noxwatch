"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { BookOpen, Cable, CircleCheck, LifeBuoy, LockKeyhole, Plus, Terminal } from "lucide-react";
import { useAuth } from "@/app/providers";
import { DashboardShell } from "@/components/dashboard-shell";
import { Button } from "@/components/ui/button";
import { type Workspace } from "@/lib/api";

export default function GuidePage() {
  const auth = useAuth();
  const router = useRouter();
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: () => auth.request<Workspace[]>("/api/v1/workspaces"), enabled: Boolean(auth.accessToken) });
  const workspace = workspaces.data?.[0];

  useEffect(() => { if (!auth.loading && !auth.user) router.replace("/login"); }, [auth.loading, auth.user, router]);

  if (auth.loading || !auth.user || workspaces.isLoading || !workspace) return <main className="min-h-screen animate-pulse bg-background" />;
  return <DashboardShell workspace={workspace} title="Guide" description="Local-first operations handbook" action={<Button onClick={() => router.push("/servers/add")}><Plus className="h-4 w-4" />Add Server</Button>}>
    <div className="mx-auto grid max-w-7xl gap-5 px-5 py-6 xl:grid-cols-[220px_minmax(0,1fr)]">
      <aside className="h-fit rounded-lg border border-panel-border bg-panel p-4 text-sm xl:sticky xl:top-24">
        <p className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted">On this page</p>
        <nav className="grid gap-1">
          <a className="rounded px-2 py-2 hover:bg-background" href="#first-run">First-time setup</a>
          <a className="rounded px-2 py-2 hover:bg-background" href="#enroll">Enroll a server</a>
          <a className="rounded px-2 py-2 hover:bg-background" href="#daily">Daily startup</a>
          <a className="rounded px-2 py-2 hover:bg-background" href="#diagnostics">Diagnostics</a>
          <a className="rounded px-2 py-2 hover:bg-background" href="#security">Security model</a>
        </nav>
      </aside>

      <div className="grid gap-5">
        <GuideSection id="first-run" icon={<BookOpen />} title="First-time local setup" intro="Prepare the local stack and install the desktop helper once.">
          <Steps items={[
            <>Copy the environment template and adjust the exposed web/API ports.</>,
            <>Start PostgreSQL and Redis, then apply migrations.</>,
            <>Install the local helper so dashboard tunnel controls work after desktop login.</>,
          ]} />
          <div className="mt-5"><Command>{`cp .env.example .env
docker compose up -d postgres redis
make migrate-up
make local-helper-install
docker compose up -d --build`}</Command></div>
        </GuideSection>

        <GuideSection id="enroll" icon={<Terminal />} title="Enroll a server" intro="NoxWatch installs the agent through your local OpenSSH client.">
          <Steps items={[
            <>Open <Link className="text-accent hover:underline" href="/servers/add">Add Server</Link> and choose <strong>SSH bootstrap</strong>.</>,
            <>Enter the SSH user, host, port, and keep <code>http://127.0.0.1:18082</code> for a reverse tunnel.</>,
            <>Choose <strong>Open in terminal</strong>, enter SSH and sudo passwords, then wait for successful enrollment.</>,
            <>The SSH tunnel moves to the background and Kitty closes automatically.</>,
            <>For a server enrolled before tunnel profiles existed, open its detail page and use <strong>Reverse tunnel → Save &amp; start</strong> once.</>,
          ]} />
        </GuideSection>

        <GuideSection id="daily" icon={<Cable />} title="Daily startup" intro="Bring the local application up, then reconnect every saved remote server.">
          <Command>{`docker compose up -d
# Open the dashboard, then click:
# Start all tunnels`}</Command>
          <p className="mt-4 text-sm leading-6 text-muted">One Kitty window prompts for servers that need password authentication. Successful tunnels continue in background; the button updates every three seconds and becomes <strong className="text-foreground">Stop all</strong> when every profile is connected.</p>
        </GuideSection>

        <GuideSection id="diagnostics" icon={<LifeBuoy />} title="Diagnostics" intro="Run these checks in order when a server stays offline.">
          <Command>{`systemctl --user status noxwatch-local-helper
curl http://127.0.0.1:8082/health

# On the monitored server:
curl http://127.0.0.1:18082/health
sudo systemctl status noxwatch-agent
sudo journalctl -u noxwatch-agent -n 50 --no-pager`}</Command>
          <p className="mt-4 text-sm leading-6 text-muted">Offline detection allows two minutes without a heartbeat. The dashboard refreshes status automatically, so a manual reload is not required.</p>
        </GuideSection>

        <GuideSection id="security" icon={<LockKeyhole />} title="Security model" intro="Tunnel automation does not weaken the existing SSH boundary.">
          <ul className="grid gap-3 text-sm text-muted">
            <Bullet>SSH and sudo passwords are entered only in the local terminal and are never stored by NoxWatch.</Bullet>
            <Bullet>Local profiles contain only server names, SSH targets, and ports in <code>~/.config/noxwatch/tunnels.json</code>.</Bullet>
            <Bullet>The helper binds to <code>127.0.0.1:9734</code> and accepts only the configured dashboard origin.</Bullet>
            <Bullet>The reverse port binds to remote loopback, not the server&apos;s public interface.</Bullet>
          </ul>
        </GuideSection>
      </div>
    </div>
  </DashboardShell>;
}

function GuideSection({ id, icon, title, intro, children }: { id: string; icon: React.ReactNode; title: string; intro: string; children: React.ReactNode }) {
  return <section id={id} className="scroll-mt-24 rounded-lg border border-panel-border bg-panel p-5 sm:p-6">
    <div className="flex items-start gap-3"><span className="mt-0.5 text-accent [&>svg]:h-5 [&>svg]:w-5">{icon}</span><div><h2 className="text-base font-semibold">{title}</h2><p className="mt-1 text-sm text-muted">{intro}</p></div></div>
    <div className="mt-5">{children}</div>
  </section>;
}

function Steps({ items }: { items: React.ReactNode[] }) {
  return <ol className="grid gap-3 text-sm">{items.map((item, index) => <li key={index} className="flex gap-3"><span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-accent/10 text-xs text-accent">{index + 1}</span><span className="pt-0.5 leading-6 text-muted">{item}</span></li>)}</ol>;
}

function Command({ children }: { children: string }) {
  return <pre className="overflow-x-auto rounded-md border border-panel-border bg-[#050c12] p-4 text-xs leading-6 text-[#b9cad9]"><code>{children}</code></pre>;
}

function Bullet({ children }: { children: React.ReactNode }) {
  return <li className="flex gap-3"><CircleCheck className="mt-0.5 h-4 w-4 shrink-0 text-accent" /><span className="leading-6">{children}</span></li>;
}

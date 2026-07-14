"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Activity, Bell, Boxes, LayoutDashboard, LogOut, Moon, Plug, Sun } from "lucide-react";
import { useAuth } from "@/app/providers";
import { type Workspace } from "@/lib/api";

const navigation = [
  { label: "Overview", href: "/", icon: LayoutDashboard },
  { label: "Servers", href: "/servers", icon: Boxes },
  { label: "Alerts", href: "/alerts", icon: Bell },
  { label: "Integrations", href: "/integrations", icon: Plug },
];

export function DashboardShell({ workspace, title, description, action, children }: { workspace: Workspace; title: string; description?: string; action?: React.ReactNode; children: React.ReactNode }) {
  const auth = useAuth();
  const router = useRouter();
  const pathname = usePathname();
  const [light, setLight] = useState(false);

  useEffect(() => {
    const next = localStorage.getItem("noxwatch-theme") === "light";
    document.documentElement.dataset.theme = next ? "light" : "dark";
    const frame = requestAnimationFrame(() => setLight(next));
    return () => cancelAnimationFrame(frame);
  }, []);

  function toggleTheme() {
    const next = !light;
    setLight(next);
    localStorage.setItem("noxwatch-theme", next ? "light" : "dark");
    document.documentElement.dataset.theme = next ? "light" : "dark";
  }

  return <main className="min-h-screen bg-background text-foreground">
    <aside className="fixed inset-y-0 left-0 hidden w-64 border-r border-panel-border bg-panel p-5 lg:flex lg:flex-col">
      <Link href="/" className="flex items-center gap-3">
        <span className="flex h-9 w-9 items-center justify-center rounded-md border border-accent/40 bg-accent/10"><Activity className="h-5 w-5 text-accent" /></span>
        <div className="min-w-0"><p className="text-sm font-semibold">NoxWatch</p><p className="truncate text-xs text-muted">{workspace.name}</p></div>
      </Link>
      <nav className="mt-10 grid gap-1 text-sm">
        {navigation.map((item) => {
          const active = item.href === "/" ? pathname === "/" : pathname === item.href || pathname.startsWith(`${item.href}/`);
          const Icon = item.icon;
          return <Link key={item.label} href={item.href} className={`flex items-center gap-3 rounded-md px-3 py-2 ${active ? "bg-background text-foreground" : "text-muted hover:bg-background/60 hover:text-foreground"}`}><Icon className="h-4 w-4" />{item.label}</Link>;
        })}
        {['Team', 'Audit Logs', 'Settings'].map((item) => <span key={item} className="flex items-center justify-between rounded-md px-3 py-2 text-muted opacity-55"><span>{item}</span><span className="text-[10px] uppercase">Planned</span></span>)}
      </nav>
      <div className="mt-auto grid gap-1">
        <button title={light ? "Use dark theme" : "Use light theme"} className="flex items-center gap-3 rounded-md px-3 py-2 text-left text-sm text-muted hover:bg-background hover:text-foreground" onClick={toggleTheme}>{light ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}{light ? "Dark theme" : "Light theme"}</button>
        <button className="flex items-center gap-3 rounded-md px-3 py-2 text-left text-sm text-muted hover:bg-background hover:text-foreground" onClick={() => auth.logout().then(() => router.replace("/login"))}><LogOut className="h-4 w-4" />Sign out</button>
      </div>
    </aside>

    <section className="lg:pl-64">
      <header className="sticky top-0 z-10 border-b border-panel-border bg-background/95 px-5 py-4 backdrop-blur">
        <div className="mx-auto flex max-w-7xl items-center justify-between gap-4">
          <div className="min-w-0"><h1 className="truncate text-xl font-semibold">{title}</h1><p className="truncate text-sm text-muted">{description ?? `${workspace.name} · ${workspace.role}`}</p></div>
          <div className="shrink-0">{action}</div>
        </div>
        <nav className="mx-auto mt-4 flex max-w-7xl gap-1 overflow-x-auto lg:hidden">
          {navigation.map((item) => <Link key={item.label} href={item.href} className={`shrink-0 rounded-md px-3 py-2 text-xs ${pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href)) ? "bg-panel text-foreground" : "text-muted"}`}>{item.label}</Link>)}
        </nav>
      </header>
      {children}
    </section>
  </main>;
}

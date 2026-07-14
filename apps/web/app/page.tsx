import { Activity, Bell, Copy, Plus, Server, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { StatusPill } from "@/components/status-pill";
import { servers, summary } from "@/lib/dashboard-data";

export default function Home() {
  return (
    <main className="min-h-screen bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 hidden w-64 border-r border-panel-border bg-[#09131d] p-5 lg:block">
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-md border border-accent/40 bg-accent/10">
            <Activity className="h-5 w-5 text-accent" aria-hidden="true" />
          </div>
          <div>
            <p className="text-sm font-semibold">NoxWatch</p>
            <p className="text-xs text-muted">Monitor every server.</p>
          </div>
        </div>
        <nav className="mt-10 grid gap-1 text-sm text-muted">
          {["Overview", "Servers", "Alerts", "Integrations", "Team", "Audit Logs", "Settings"].map((item) => (
            <a key={item} className="rounded-md px-3 py-2 hover:bg-panel hover:text-foreground" href="#">
              {item}
            </a>
          ))}
        </nav>
      </aside>

      <section className="lg:pl-64">
        <header className="sticky top-0 z-10 border-b border-panel-border bg-background/95 px-5 py-4 backdrop-blur">
          <div className="mx-auto flex max-w-7xl items-center justify-between gap-4">
            <div>
              <h1 className="text-xl font-semibold tracking-normal">Overview</h1>
              <p className="text-sm text-muted">Workspace foundation preview. Live data starts after enrollment.</p>
            </div>
            <Button>
              <Plus className="h-4 w-4" aria-hidden="true" />
              Add Server
            </Button>
          </div>
        </header>

        <div className="mx-auto grid max-w-7xl gap-5 px-5 py-6">
          <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            {summary.map((item) => (
              <div key={item.label} className="rounded-lg border border-panel-border bg-panel p-4">
                <p className="text-sm text-muted">{item.label}</p>
                <p className="mt-2 text-3xl font-semibold">{item.value}</p>
                <p className="mt-1 text-sm text-muted">{item.detail}</p>
              </div>
            ))}
          </section>

          <section className="grid gap-5 xl:grid-cols-[1.4fr_0.8fr]">
            <div className="rounded-lg border border-panel-border bg-panel">
              <div className="flex items-center justify-between border-b border-panel-border p-4">
                <div>
                  <h2 className="text-sm font-semibold">Servers</h2>
                  <p className="text-sm text-muted">Seed-style preview data, not live monitoring.</p>
                </div>
                <Server className="h-5 w-5 text-muted" aria-hidden="true" />
              </div>
              <div className="overflow-x-auto">
                <table className="w-full min-w-[680px] border-collapse text-left text-sm">
                  <thead className="text-muted">
                    <tr>
                      <th className="px-4 py-3 font-medium">Name</th>
                      <th className="px-4 py-3 font-medium">Environment</th>
                      <th className="px-4 py-3 font-medium">Status</th>
                      <th className="px-4 py-3 font-medium">CPU</th>
                      <th className="px-4 py-3 font-medium">Memory</th>
                    </tr>
                  </thead>
                  <tbody>
                    {servers.map((server) => (
                      <tr key={server.name} className="border-t border-panel-border">
                        <td className="px-4 py-4">
                          <p className="font-medium">{server.name}</p>
                          <p className="text-xs text-muted">{server.host}</p>
                        </td>
                        <td className="px-4 py-4 text-muted">{server.env}</td>
                        <td className="px-4 py-4">
                          <StatusPill status={server.status} />
                        </td>
                        <td className="px-4 py-4">{server.cpu}%</td>
                        <td className="px-4 py-4">{server.mem}%</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>

            <div className="grid gap-5">
              <div className="rounded-lg border border-panel-border bg-panel p-4">
                <div className="flex items-center gap-2">
                  <ShieldCheck className="h-5 w-5 text-accent" aria-hidden="true" />
                  <h2 className="text-sm font-semibold">Secure Enrollment</h2>
                </div>
                <p className="mt-3 text-sm leading-6 text-muted">
                  Phase 3 will generate short-lived, one-time tokens and return permanent agent credentials only after enrollment.
                </p>
                <div className="mt-4 rounded-md border border-panel-border bg-[#071019] p-3 font-mono text-xs text-muted">
                  curl -fsSL https://noxwatch.example.com/install.sh | sudo bash -s -- --token nox_enroll_...
                </div>
                <Button className="mt-4 w-full" variant="secondary">
                  <Copy className="h-4 w-4" aria-hidden="true" />
                  Planned Copy Command
                </Button>
              </div>

              <div className="rounded-lg border border-panel-border bg-panel p-4">
                <div className="flex items-center gap-2">
                  <Bell className="h-5 w-5 text-warning" aria-hidden="true" />
                  <h2 className="text-sm font-semibold">Alerts</h2>
                </div>
                <p className="mt-3 text-sm leading-6 text-muted">
                  Alert rules, lifecycle records, and signed webhook notifications start in Phase 6.
                </p>
              </div>
            </div>
          </section>
        </div>
      </section>
    </main>
  );
}

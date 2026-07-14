import { cn } from "@/lib/utils";

type Status = "online" | "warning" | "degraded" | "offline" | "unknown" | "maintenance";

const styles: Record<Status, string> = {
  online: "border-accent/40 bg-accent/10 text-accent",
  warning: "border-warning/40 bg-warning/10 text-warning",
  degraded: "border-warning/40 bg-warning/10 text-warning",
  offline: "border-danger/40 bg-danger/10 text-danger",
  unknown: "border-muted/30 bg-muted/10 text-muted",
  maintenance: "border-muted/30 bg-muted/10 text-muted",
};

export function StatusPill({ status }: { status: Status }) {
  return (
    <span className={cn("rounded-full border px-2.5 py-1 text-xs font-medium capitalize", styles[status])}>
      {status}
    </span>
  );
}

export type User = { id: string; email: string; name: string };
export type Workspace = { id: string; name: string; slug: string; role: string };
export type AuthData = { user: User; access_token: string; expires_in: number };
export type ServerRecord = {
  id: string; workspace_id: string; name: string; hostname: string; description: string; environment: string;
  operating_system: string; os_version: string; kernel_version: string; architecture: string; agent_version: string;
  status: "online" | "degraded" | "warning" | "offline" | "unknown" | "maintenance";
  last_seen_at: string | null; enrolled_at: string | null; tags: string[]; agent_revoked: boolean;
  cpu_usage_percent: number | null; memory_usage_percent: number | null; disk_usage_percent: number | null; uptime_seconds: number | null;
  ssh_user?: string; ssh_host?: string; ssh_port?: number; tunnel_remote_port?: number;
};
export type MetricSample = {
  collected_at: string; uptime_seconds: number; process_count: number; cpu_usage_percent: number;
  load_1: number; load_5: number; load_15: number; memory_total_bytes: number; memory_used_bytes: number;
  memory_usage_percent: number; swap_total_bytes: number; swap_used_bytes: number; swap_usage_percent: number;
};
export type MetricSnapshot = MetricSample & {
  disks: Array<{ mount_point: string; filesystem: string; total_bytes: number; used_bytes: number; available_bytes: number; usage_percent: number; inode_usage_percent: number }>;
  networks: Array<{ interface: string; rx_bytes_total: number; tx_bytes_total: number; rx_bytes_per_second: number; tx_bytes_per_second: number }>;
};
export type AlertRule = {
  id: string; workspace_id: string; server_id: string; name: string;
  metric: "cpu_usage" | "memory_usage" | "disk_usage" | "swap_usage" | "server_offline" | "agent_disconnected";
  warning_threshold: number | null; critical_threshold: number | null;
  evaluation_seconds: number; cooldown_seconds: number; enabled: boolean;
};
export type AlertEvent = {
  id: string; alert_rule_id: string; server_id: string; rule_name: string; severity: "warning" | "critical";
  state: "pending" | "firing" | "resolved" | "acknowledged"; current_value: number; threshold: number;
  triggered_at: string; resolved_at: string | null;
};
export type NotificationChannel = { id: string; name: string; type: "webhook"; enabled: boolean; created_at: string; secret?: string };

type Envelope<T> = {
  data: T | null;
  error: { code: string; message: string; fields?: Record<string, string> } | null;
};

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  constructor(
    message: string,
    public code = "REQUEST_FAILED",
    public fields?: Record<string, string>,
  ) {
    super(message);
  }
}

export async function api<T>(path: string, init: RequestInit = {}, accessToken?: string): Promise<T> {
  const response = await fetch(`${API_URL}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      ...(init.body ? { "Content-Type": "application/json" } : {}),
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      ...init.headers,
    },
  });
  const body = (await response.json().catch(() => null)) as Envelope<T> | null;
  if (!response.ok || !body?.data) {
    throw new ApiError(body?.error?.message ?? "NoxWatch could not complete the request.", body?.error?.code, body?.error?.fields);
  }
  return body.data;
}

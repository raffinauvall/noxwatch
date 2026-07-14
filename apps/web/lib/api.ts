export type User = { id: string; email: string; name: string };
export type Workspace = { id: string; name: string; slug: string; role: string };
export type AuthData = { user: User; access_token: string; expires_in: number };
export type ServerRecord = {
  id: string; workspace_id: string; name: string; hostname: string; description: string; environment: string;
  operating_system: string; os_version: string; kernel_version: string; architecture: string; agent_version: string;
  status: "online" | "degraded" | "warning" | "offline" | "unknown" | "maintenance";
  last_seen_at: string | null; enrolled_at: string | null; tags: string[]; agent_revoked: boolean;
  cpu_usage_percent: number | null; memory_usage_percent: number | null; disk_usage_percent: number | null; uptime_seconds: number | null;
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

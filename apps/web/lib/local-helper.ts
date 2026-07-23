export type TunnelStatus = {
  id: string;
  server_id?: string;
  name: string;
  target: string;
  port: string;
  local_port: string;
  remote_port: string;
  running: boolean;
};

export async function localHelper<T>(path: string, method = "GET", body?: object): Promise<T> {
  const response = await fetch(`http://127.0.0.1:9734${path}`, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const result = await response.json().catch(() => ({})) as T & { error?: string };
  if (!response.ok) throw new Error(result.error || "Local tunnel helper is unavailable.");
  return result;
}

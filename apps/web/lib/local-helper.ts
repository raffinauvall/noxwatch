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

export async function syncTunnelProfile(server: ServerRecord, profiles: TunnelStatus[], request: (path: string, init?: RequestInit) => Promise<unknown>) {
  const cached = profiles.find((profile) => profile.server_id === server.id || profile.id === server.id);
  let sshUser = server.ssh_user;
  let sshHost = server.ssh_host;
  let sshPort = server.ssh_port;
  let remotePort = server.tunnel_remote_port;
  if ((!sshUser || !sshHost || !sshPort || !remotePort) && cached) {
    const separator = cached.target.indexOf("@");
    sshUser = cached.target.slice(0, separator);
    sshHost = cached.target.slice(separator + 1);
    sshPort = Number(cached.port);
    remotePort = Number(cached.remote_port);
    await request(`/api/v1/servers/${server.id}/tunnel`, { method: "PUT", body: JSON.stringify({ ssh_user: sshUser, ssh_host: sshHost, ssh_port: sshPort, remote_port: remotePort }) });
  }
  if (!sshUser || !sshHost || !sshPort || !remotePort) return null;
  const id = cached?.id ?? server.id;
  await localHelper("/tunnels/register", "POST", { id, server_id: server.id, name: server.name, target: `${sshUser}@${sshHost}`, port: String(sshPort), remote_port: String(remotePort) });
  return id;
}
import { type ServerRecord } from "@/lib/api";

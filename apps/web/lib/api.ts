export type User = { id: string; email: string; name: string };
export type Workspace = { id: string; name: string; slug: string; role: string };
export type AuthData = { user: User; access_token: string; expires_in: number };

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

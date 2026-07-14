"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { api, type AuthData, type User } from "@/lib/api";

type AuthContextValue = {
  user: User | null;
  accessToken: string | null;
  loading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: 1, staleTime: 15_000 } } }));
  const [session, setSession] = useState<AuthData | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api<AuthData>("/api/v1/auth/refresh", { method: "POST" })
      .then(setSession)
      .catch(() => setSession(null))
      .finally(() => setLoading(false));
  }, []);

  const authenticate = useCallback(async (path: string, body: object) => {
    const next = await api<AuthData>(path, { method: "POST", body: JSON.stringify(body) });
    setSession(next);
  }, []);

  const value = useMemo<AuthContextValue>(() => ({
    user: session?.user ?? null,
    accessToken: session?.access_token ?? null,
    loading,
    login: (email, password) => authenticate("/api/v1/auth/login", { email, password }),
    register: (email, password, name) => authenticate("/api/v1/auth/register", { email, password, name }),
    logout: async () => {
      if (session?.access_token) {
        await api("/api/v1/auth/logout", { method: "POST" }, session.access_token).catch(() => undefined);
      }
      setSession(null);
      queryClient.clear();
    },
  }), [authenticate, loading, queryClient, session]);

  return <QueryClientProvider client={queryClient}><AuthContext.Provider value={value}>{children}</AuthContext.Provider></QueryClientProvider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside Providers");
  return value;
}

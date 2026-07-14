"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { ArrowRight, Building2, LoaderCircle } from "lucide-react";
import { z } from "zod";
import { useForm } from "react-hook-form";
import { useAuth } from "@/app/providers";
import { type Workspace } from "@/lib/api";
import { Button } from "@/components/ui/button";

const schema = z.object({ name: z.string().trim().min(2, "Use at least 2 characters.").max(100) });
type Values = z.infer<typeof schema>;

export default function OnboardingPage() {
  const auth = useAuth();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [requestError, setRequestError] = useState("");
  const { register, handleSubmit, setError, formState: { errors, isSubmitting } } = useForm<Values>();

  useEffect(() => {
    if (!auth.loading && !auth.user) router.replace("/login");
  }, [auth.loading, auth.user, router]);

  const submit = handleSubmit(async (values) => {
    const parsed = schema.safeParse(values);
    if (!parsed.success) {
      setError("name", { message: parsed.error.issues[0].message });
      return;
    }
    setRequestError("");
    try {
      await auth.request<Workspace>("/api/v1/workspaces", { method: "POST", body: JSON.stringify(parsed.data) });
      await queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      router.replace("/");
    } catch (error) {
      setRequestError(error instanceof Error ? error.message : "Workspace creation failed.");
    }
  });

  if (auth.loading || !auth.user) return <main className="min-h-screen bg-background" />;
  return <main className="grid min-h-screen place-items-center bg-background px-6 text-foreground">
    <div className="w-full max-w-md">
      <span className="flex h-11 w-11 items-center justify-center rounded-md border border-accent/30 bg-accent/10"><Building2 className="h-5 w-5 text-accent" /></span>
      <p className="mt-8 font-mono text-xs uppercase text-accent">Workspace setup</p>
      <h1 className="mt-3 text-3xl font-semibold">Name your operations space</h1>
      <p className="mt-3 text-sm leading-6 text-muted">Servers, metrics, alerts, and enrollment credentials stay isolated inside this workspace.</p>
      <form className="mt-8 grid gap-3" onSubmit={submit}>
        <label className="text-sm font-medium" htmlFor="workspace-name">Workspace name</label>
        <input id="workspace-name" className="h-11 rounded-md border border-panel-border bg-panel px-3 outline-none focus:border-accent" autoFocus {...register("name")} />
        {errors.name?.message && <p className="text-xs text-danger">{errors.name.message}</p>}
        {requestError && <p className="text-sm text-danger" role="alert">{requestError}</p>}
        <Button className="mt-4" type="submit" disabled={isSubmitting}>{isSubmitting ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <ArrowRight className="h-4 w-4" />}Create workspace</Button>
      </form>
    </div>
  </main>;
}

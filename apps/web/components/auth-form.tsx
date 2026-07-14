"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { cloneElement, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Activity, ArrowRight, LoaderCircle } from "lucide-react";
import { useAuth } from "@/app/providers";
import { ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";

const schema = z.object({
  name: z.string().trim().max(100).optional(),
  email: z.email("Enter a valid email address."),
  password: z.string().min(12, "Use at least 12 characters.").max(128),
});

type FormValues = z.infer<typeof schema>;

export function AuthForm({ mode }: { mode: "login" | "register" }) {
  const router = useRouter();
  const auth = useAuth();
  const { register, handleSubmit, setError, formState: { errors, isSubmitting } } = useForm<FormValues>();
  const creating = mode === "register";

  const submit = handleSubmit(async (values) => {
    const parsed = schema.safeParse(values);
    if (!parsed.success) {
      for (const issue of parsed.error.issues) setError(issue.path[0] as keyof FormValues, { message: issue.message });
      return;
    }
    try {
      if (creating) await auth.register(parsed.data.email, parsed.data.password, parsed.data.name ?? "");
      else await auth.login(parsed.data.email, parsed.data.password);
      router.replace("/");
    } catch (error) {
      if (error instanceof ApiError && error.fields) {
        for (const [field, message] of Object.entries(error.fields)) setError(field as keyof FormValues, { message });
      }
      setError("root", { message: error instanceof Error ? error.message : "Authentication failed." });
    }
  });

  return (
    <main className="grid min-h-screen lg:grid-cols-[1fr_440px]">
      <section className="hidden border-r border-panel-border bg-[#09131d] p-12 lg:flex lg:flex-col lg:justify-between">
        <Link href="/" className="flex items-center gap-3 text-sm font-semibold">
          <span className="flex h-9 w-9 items-center justify-center rounded-md border border-accent/40 bg-accent/10">
            <Activity className="h-5 w-5 text-accent" aria-hidden="true" />
          </span>
          NoxWatch
        </Link>
        <div className="max-w-xl">
          <p className="mb-5 font-mono text-xs uppercase text-accent">Infrastructure visibility</p>
          <h1 className="text-5xl font-semibold leading-tight">Monitor every server.<br />Miss nothing.</h1>
          <p className="mt-6 max-w-lg text-base leading-7 text-muted">A focused control surface for Linux health, resource pressure, and agent availability.</p>
        </div>
        <p className="text-xs text-muted">Outbound-only agents. Revocable sessions. Workspace isolation.</p>
      </section>

      <section className="flex items-center justify-center px-6 py-12">
        <div className="w-full max-w-sm">
          <Link href="/" className="mb-12 flex items-center gap-2 text-sm font-semibold lg:hidden"><Activity className="h-5 w-5 text-accent" />NoxWatch</Link>
          <h2 className="text-2xl font-semibold">{creating ? "Create your account" : "Welcome back"}</h2>
          <p className="mt-2 text-sm text-muted">{creating ? "Start with a secure operations workspace." : "Continue to your infrastructure workspace."}</p>

          <form className="mt-8 grid gap-5" onSubmit={submit} noValidate>
            {creating && <Field label="Name" error={errors.name?.message}><input autoComplete="name" {...register("name")} /></Field>}
            <Field label="Email" error={errors.email?.message}><input type="email" autoComplete="email" {...register("email")} /></Field>
            <Field label="Password" error={errors.password?.message}><input type="password" autoComplete={creating ? "new-password" : "current-password"} {...register("password")} /></Field>
            {errors.root?.message && <p className="rounded-md border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger" role="alert">{errors.root.message}</p>}
            <Button className="mt-1 w-full" type="submit" disabled={isSubmitting}>
              {isSubmitting ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <ArrowRight className="h-4 w-4" />}
              {creating ? "Create account" : "Sign in"}
            </Button>
          </form>

          <p className="mt-7 text-center text-sm text-muted">
            {creating ? "Already have an account?" : "New to NoxWatch?"}{" "}
            <Link className="font-medium text-foreground hover:text-accent" href={creating ? "/login" : "/register"}>{creating ? "Sign in" : "Create account"}</Link>
          </p>
        </div>
      </section>
    </main>
  );
}

function Field({ label, error, children }: { label: string; error?: string; children: ReactElement<{ className?: string; "aria-invalid"?: boolean }>; }) {
  return <label className="grid gap-2 text-sm font-medium">{label}{cloneElement(children, {
    className: "h-11 rounded-md border border-panel-border bg-panel px-3 text-foreground outline-none transition focus:border-accent",
    "aria-invalid": Boolean(error),
  })}{error && <span className="text-xs font-normal text-danger">{error}</span>}</label>;
}

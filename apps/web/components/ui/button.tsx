import * as React from "react";
import { cn } from "@/lib/utils";

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary";
};

export function Button({ className, variant = "primary", ...props }: ButtonProps) {
  return (
    <button
      className={cn(
        "inline-flex h-10 items-center justify-center gap-2 rounded-md px-4 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-accent disabled:cursor-not-allowed disabled:opacity-50",
        variant === "primary"
          ? "bg-accent text-[#04100d] hover:bg-[#7ef0d5]"
          : "border border-panel-border bg-panel text-foreground hover:bg-[#142232]",
        className,
      )}
      {...props}
    />
  );
}

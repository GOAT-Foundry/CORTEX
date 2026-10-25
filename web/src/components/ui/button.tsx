import * as React from "react";
import { cn } from "@/lib/utils";

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "default" | "outline" | "ghost";
  size?: "default" | "sm";
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "default", size = "default", ...props }, ref) => {
    return (
      <button
        ref={ref}
        className={cn(
          "inline-flex items-center justify-center gap-2 rounded-md text-sm font-medium transition-colors focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50 cursor-pointer",
          variant === "default" &&
            "bg-sky-500 text-zinc-950 hover:bg-sky-400 shadow-sm",
          variant === "outline" &&
            "border border-zinc-800 bg-zinc-900/70 text-zinc-100 hover:bg-zinc-800",
          variant === "ghost" &&
            "text-zinc-300 hover:bg-zinc-800/70 hover:text-zinc-100",
          size === "default" && "h-9 px-4 py-2",
          size === "sm" && "h-8 px-3 text-xs",
          className
        )}
        {...props}
      />
    );
  }
);
Button.displayName = "Button";

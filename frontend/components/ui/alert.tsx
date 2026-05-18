import type { HTMLAttributes } from "react";
import { cn } from "@/lib/utils";

type AlertVariant = "default" | "destructive";

type AlertProps = HTMLAttributes<HTMLDivElement> & {
  variant?: AlertVariant;
};

export function Alert({ className, variant = "default", ...props }: AlertProps) {
  return <div className={cn("ui-alert", `ui-alert-${variant}`, className)} role="alert" {...props} />;
}

export function AlertTitle({ className, ...props }: HTMLAttributes<HTMLHeadingElement>) {
  return <h3 className={cn("ui-alert-title", className)} {...props} />;
}

export function AlertDescription({ className, ...props }: HTMLAttributes<HTMLParagraphElement>) {
  return <p className={cn("ui-alert-description", className)} {...props} />;
}

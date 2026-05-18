import type { FormHTMLAttributes, HTMLAttributes, LabelHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

export function Form({ className, ...props }: FormHTMLAttributes<HTMLFormElement>) {
  return <form className={cn("ui-form", className)} {...props} />;
}

export function FormItem({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("ui-form-item", className)} {...props} />;
}

export function FormLabel({ className, ...props }: LabelHTMLAttributes<HTMLLabelElement>) {
  return <label className={cn("ui-form-label", className)} {...props} />;
}

export function FormControl({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("ui-form-control", className)} {...props} />;
}

export function FormDescription({ className, ...props }: HTMLAttributes<HTMLParagraphElement>) {
  return <p className={cn("ui-form-description", className)} {...props} />;
}

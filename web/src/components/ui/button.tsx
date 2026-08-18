import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center whitespace-nowrap rounded-xl text-sm font-semibold ring-offset-background transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default: "bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-primary-foreground shadow-[0_14px_34px_rgba(79,107,237,0.38),inset_0_1px_0_rgba(255,255,255,0.35)] hover:brightness-110",
        destructive: "bg-gradient-to-br from-[#E5484D] to-[#C4372C] text-destructive-foreground shadow-[0_10px_26px_rgba(229,72,77,0.3)] hover:brightness-110",
        outline: "glass-soft text-foreground hover:bg-white/90",
        secondary: "bg-white/70 text-secondary-foreground hover:bg-white/90",
        ghost: "hover:bg-white/60 text-foreground",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: { default: "h-10 px-4 py-2", sm: "h-9 rounded-lg px-3", lg: "h-11 rounded-xl px-8", icon: "h-10 w-10" },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> { asChild?: boolean }

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return <Comp className={cn(buttonVariants({ variant, size, className }))} ref={ref} {...props} />;
  },
);
Button.displayName = "Button";
export { Button, buttonVariants };

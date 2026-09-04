import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva("inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2", {
  variants: {
    variant: {
      default: "bg-gradient-to-br from-[#F78B28] to-[#E85D3F] text-primary-foreground shadow-[0_8px_20px_rgba(247,139,40,0.28)]",
      secondary: "glass-soft text-secondary-foreground",
      destructive: "bg-[#E5484D]/12 text-[#C4372C] before:content-[''] before:inline-block before:w-1.5 before:h-1.5 before:rounded-full before:bg-[#E5484D] before:shadow-[0_0_8px_#E5484D]",
      outline: "glass-soft text-foreground",
      success: "bg-[#1BA878]/12 text-[#0C7A55] before:content-[''] before:inline-block before:w-1.5 before:h-1.5 before:rounded-full before:bg-[#1BA878] before:shadow-[0_0_8px_#1BA878]",
      warning: "bg-[#E9A23B]/14 text-[#A06B12] before:content-[''] before:inline-block before:w-1.5 before:h-1.5 before:rounded-full before:bg-[#E9A23B] before:shadow-[0_0_8px_#E9A23B]",
    },
  },
  defaultVariants: { variant: "default" },
});

export interface BadgeProps extends React.HTMLAttributes<HTMLDivElement>, VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />;
}
export { Badge, badgeVariants };

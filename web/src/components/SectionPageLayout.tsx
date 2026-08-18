import React from "react";
import { cn } from "@/lib/utils";

type SectionPageLayoutProps = React.HTMLAttributes<HTMLDivElement>;
function SectionPageLayout({ className, children, ...props }: SectionPageLayoutProps) {
  return <div className={cn("space-y-6", className)} {...props}>{children}</div>;
}

function PageTitle({ children, className }: { children: React.ReactNode; className?: string }) {
  return <h2 className={cn("font-display text-[25px] font-bold tracking-tight", className)}>{children}</h2>;
}

function PageDescription({ children, className }: { children: React.ReactNode; className?: string }) {
  return <p className={cn("text-[13px] text-[#5C6472] mt-1", className)}>{children}</p>;
}

function PageHeader({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn("flex items-center justify-between", className)}>{children}</div>;
}

function PageHeaderBlock({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn("", className)}>{children}</div>;
}

function PageActions({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn("flex items-center gap-2", className)}>{children}</div>;
}

function PageContent({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn("", className)}>{children}</div>;
}

SectionPageLayout.Title = PageTitle;
SectionPageLayout.Description = PageDescription;
SectionPageLayout.Header = PageHeader;
SectionPageLayout.HeaderBlock = PageHeaderBlock;
SectionPageLayout.Actions = PageActions;
SectionPageLayout.Content = PageContent;

export { SectionPageLayout };

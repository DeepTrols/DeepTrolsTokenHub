import React from "react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { AlertCircle, Inbox, Loader2, type LucideIcon } from "lucide-react";

type EmptyStateProps = {
  icon?: LucideIcon; title: string; description?: string; action?: React.ReactNode; className?: string;
};
function EmptyState({ icon: Icon = Inbox, title, description, action, className }: EmptyStateProps) {
  return (
    <Card className={cn("", className)}>
      <CardContent className="flex flex-col items-center justify-center py-12 text-center">
        <Icon size={40} className="text-muted-foreground/30 mb-3" />
        <p className="text-muted-foreground font-medium">{title}</p>
        {description && <p className="text-sm text-muted-foreground/70 mt-1">{description}</p>}
        {action && <div className="mt-4">{action}</div>}
      </CardContent>
    </Card>
  );
}

type ErrorStateProps = { error?: unknown; onRetry?: () => void; title?: string; className?: string };
function ErrorState({ error, onRetry, title = "加载失败", className }: ErrorStateProps) {
  const message = error instanceof Error ? error.message : error ? String(error) : undefined;
  return (
    <Card className={cn("border-destructive/20", className)}>
      <CardContent className="flex flex-col items-center py-8 text-center">
        <AlertCircle size={32} className="text-destructive/70 mb-3" />
        <p className="font-medium text-destructive">{title}</p>
        {message && <p className="text-sm text-muted-foreground mt-1 max-w-md">{message}</p>}
        {onRetry && <Button variant="destructive" size="sm" className="mt-4" onClick={onRetry}>重试</Button>}
      </CardContent>
    </Card>
  );
}

type LoadingStateProps = { message?: string; className?: string };
function LoadingState({ message = "加载中...", className }: LoadingStateProps) {
  return (
    <Card className={cn("", className)}>
      <CardContent className="flex flex-col items-center justify-center py-12 text-center">
        <Loader2 size={32} className="animate-spin text-primary mb-3" />
        <p className="text-muted-foreground">{message}</p>
      </CardContent>
    </Card>
  );
}

export { EmptyState, ErrorState, LoadingState };

import { useSiteInfo } from "@/lib/site";

export function SiteFooter({ className }: { className?: string }) {
  const { site } = useSiteInfo();
  if (!site.footer_text) return null;
  return (
    <div className={className || "pt-6 pb-2 text-center text-xs text-muted-foreground"}>
      {site.footer_text}
    </div>
  );
}

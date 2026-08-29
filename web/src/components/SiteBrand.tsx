import { useSiteInfo } from "@/lib/site";

export function SiteBrand({ className }: { className?: string }) {
  const { site } = useSiteInfo();
  return (
    <img
      src={site.logo_url || "/brand-logo.png"}
      alt={site.site_name || "DEEPTROLS"}
      className={className || "w-[168px] h-auto"}
    />
  );
}

import { useSiteInfo } from "@/lib/site";

interface SiteBrandProps {
  className?: string;
  imageClassName?: string;
  textClassName?: string;
}

export function SiteBrand({ className, imageClassName, textClassName }: SiteBrandProps) {
  const { site } = useSiteInfo();
  const brandName = site.site_name || "智曜TokenHub";

  return (
    <div className={className || "flex min-w-0 items-center gap-2"}>
      <img
        src={site.logo_url || "/brand-logo.png"}
        alt=""
        className={imageClassName || "h-9 w-[30px] shrink-0 rounded-lg object-contain"}
      />
      <span className={textClassName || "truncate text-[17px] font-bold leading-none text-[#161A23]"}>
        {brandName}
      </span>
    </div>
  );
}

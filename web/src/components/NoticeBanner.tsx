import { useSiteInfo } from "@/lib/site";

export function NoticeBanner() {
  const { site } = useSiteInfo();
  if (!site.notice) return null;
  return (
    <div className="glass-soft rounded-xl px-4 py-2.5 text-[13px] text-[#4F6BED] font-medium">
      {site.notice}
    </div>
  );
}

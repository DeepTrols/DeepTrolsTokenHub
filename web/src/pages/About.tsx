import { useSiteInfo } from "@/lib/site";
import "../i18n";
import { useTranslation } from "react-i18next";

export default function About() {
  const { t } = useTranslation();
  const { site } = useSiteInfo();
  return (
    <div className="mx-auto max-w-2xl p-6 space-y-4">
      <h1 className="font-display text-2xl font-bold">{site.site_name} · {t("about.about")}</h1>
      <div className="glass rounded-2xl p-6 text-sm whitespace-pre-wrap text-foreground/90">
        {site.about || t("about.defaultDesc")}
      </div>
    </div>
  );
}

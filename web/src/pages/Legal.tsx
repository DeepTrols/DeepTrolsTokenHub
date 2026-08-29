import { useSiteInfo } from "@/lib/site";
import "../i18n";
import { useTranslation } from "react-i18next";

export default function Legal({ kind }: { kind: "user_agreement" | "privacy_policy" }) {
  const { t } = useTranslation();
  const { site } = useSiteInfo();
  const content = kind === "user_agreement" ? site.legal.user_agreement : site.legal.privacy_policy;
  const title = kind === "user_agreement" ? t("legal.userAgreement") : t("legal.privacyPolicy");
  return (
    <div className="mx-auto max-w-2xl p-6 space-y-4">
      <h1 className="font-display text-2xl font-bold">{site.site_name} · {title}</h1>
      <div className="glass rounded-2xl p-6 text-sm whitespace-pre-wrap text-foreground/90">
        {content || t("legal.notConfigured")}
      </div>
    </div>
  );
}

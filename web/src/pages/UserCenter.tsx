import { ProfileContent } from "./Profile";
import "../i18n";
import { useTranslation } from "react-i18next";

export default function UserCenter() {
  const { t } = useTranslation();
  return (
    <div>
      <div className="mb-6">
        <h2 className="font-display text-[25px] font-bold tracking-tight">{t("usercenter.title")}</h2>
        <p className="text-[13px] text-[#5C6472] mt-1">{t("usercenter.subtitle")}</p>
      </div>

      <ProfileContent />
    </div>
  );
}

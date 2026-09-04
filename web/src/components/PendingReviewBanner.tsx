import { Info } from "lucide-react";
import "../i18n";
import { useTranslation } from "react-i18next";

interface PendingReviewBannerProps {
  /** Tenant lifecycle status from /me; empty for personal users. */
  tenantStatus?: string;
}

/**
 * Shows a persistent notice while an enterprise tenant awaits platform-admin
 * approval. Renders nothing for personal users or non-pending tenant states.
 */
export function PendingReviewBanner({ tenantStatus }: PendingReviewBannerProps) {
  const { t } = useTranslation();
  if (tenantStatus !== "pending_review") return null;

  return (
    <div
      role="status"
      className="mb-6 flex items-start gap-3 rounded-2xl glass-soft border-[#E9A23B]/30 p-4 text-sm"
    >
      <Info size={18} className="mt-0.5 shrink-0 text-[#E9A23B]" />
      <div>
        <p className="font-medium text-[#A06B12]">{t("components.pendingTitle")}</p>
        <p className="mt-0.5 text-[#A06B12]/85">{t("components.pendingDesc")}</p>
      </div>
    </div>
  );
}

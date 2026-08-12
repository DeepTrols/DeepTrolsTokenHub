import { Info } from "lucide-react";

interface PendingReviewBannerProps {
  /** Tenant lifecycle status from /me; empty for personal users. */
  tenantStatus?: string;
}

/**
 * Shows a persistent notice while an enterprise tenant awaits platform-admin
 * approval. Renders nothing for personal users or non-pending tenant states.
 */
export function PendingReviewBanner({ tenantStatus }: PendingReviewBannerProps) {
  if (tenantStatus !== "pending_review") return null;

  return (
    <div
      role="status"
      className="mb-6 flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm"
    >
      <Info size={18} className="mt-0.5 shrink-0 text-amber-600" />
      <div>
        <p className="font-medium text-amber-900">企业账号审核中</p>
        <p className="mt-0.5 text-amber-800">
          您的企业账号正在等待平台管理员审核，审核通过后将自动开放团队管理与配额分配等功能。
        </p>
      </div>
    </div>
  );
}

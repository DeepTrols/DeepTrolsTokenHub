import { QRCodeSVG } from "qrcode.react";
import { Loader2, CheckCircle2 } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import "../i18n";
import { useTranslation } from "react-i18next";

interface Props {
  open: boolean;
  onClose: () => void;
  payURL: string;
  orderNo: string;
  paid: boolean;
}

export function PaymentQrDialog({ open, onClose, payURL, orderNo, paid }: Props) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-[380px]">
        <DialogHeader>
          <DialogTitle>{paid ? t("components.qrSuccess") : t("components.qrScan")}</DialogTitle>
          <DialogDescription>{t("components.qrOrderNo", { no: orderNo })}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col items-center gap-3 py-4">
          {paid ? (
            <div className="flex flex-col items-center gap-3 py-6">
              <CheckCircle2 className="text-[#1BA878]" size={44} />
              <p className="text-sm text-[#0C7A55] font-medium">{t("components.qrCredited")}</p>
            </div>
          ) : (
            <>
              {payURL ? (
                <QRCodeSVG value={payURL} size={220} />
              ) : (
                <Loader2 className="animate-spin text-[#4F6BED]" size={32} />
              )}
              <p className="text-xs text-[#5C6472] text-center">
                {t("components.qrScanHint")}
                <br />
                {t("components.qrAutoRefresh")}
              </p>
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

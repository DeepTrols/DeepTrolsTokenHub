import { useState } from "react";
import { Check, ChevronDown } from "lucide-react";
import "../i18n";
import { useTranslation } from "react-i18next";

export interface SelectOption {
  value: string;
  label: string;
}

export interface SelectMenuProps {
  value: string;
  options: SelectOption[];
  onChange: (value: string) => void;
  ariaLabel: string;
  placeholder?: string;
}

export default function SelectMenu({ value, options, onChange, ariaLabel, placeholder }: SelectMenuProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const selected = options.find((o) => o.value === value);

  return (
    <div className="relative">
      <button
        type="button"
        aria-label={ariaLabel}
        onClick={() => setOpen((v) => !v)}
        className="glass-soft rounded-lg px-3 py-2 text-sm inline-flex items-center gap-2 focus:outline-none focus:border-[#4F6BED] focus:ring-2 focus:ring-[#4F6BED]/20"
      >
        <span className="max-w-[220px] truncate">{selected?.label ?? placeholder ?? t("components.selectPlaceholder")}</span>
        <ChevronDown size={14} className="text-[#5C6472] shrink-0" />
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-30" onClick={() => setOpen(false)} />
          <div className="absolute left-0 top-full mt-1 z-40 min-w-[180px] max-w-[280px] rounded-lg bg-white border border-black/10 shadow-lg py-1">
            {options.map((o) => (
              <button
                key={o.value}
                type="button"
                onClick={() => {
                  onChange(o.value);
                  setOpen(false);
                }}
                className={`w-full flex items-center justify-between gap-2 px-3 py-2 text-left text-[13px] transition-colors hover:bg-black/5 ${
                  o.value === value ? "font-semibold text-[#161A23]" : "text-[#5C6472]"
                }`}
              >
                <span className="truncate">{o.label}</span>
                {o.value === value && <Check size={14} className="text-[#4F6BED] shrink-0" />}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

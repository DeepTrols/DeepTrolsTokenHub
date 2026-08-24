import { useEffect, useMemo, useState } from "react";
import { CalendarDays, Check, ChevronDown, ChevronLeft, ChevronRight } from "lucide-react";
import {
  PresetKey,
  dayKey,
  dayKeyToEnd,
  dayKeyToStart,
  formatRangeLabel,
  gmt8DayKey,
  monthKeyFromInstant,
  rangeForPreset,
} from "../lib/gmt8";

const PRESETS: { key: PresetKey; label: string }[] = [
  { key: "today", label: "今天" },
  { key: "yesterday", label: "昨天" },
  { key: "7d", label: "近 7 天" },
  { key: "30d", label: "近 30 天" },
  { key: "month", label: "本月" },
  { key: "lastMonth", label: "上月" },
  { key: "custom", label: "自定义" },
];

const WEEKDAYS = ["日", "一", "二", "三", "四", "五", "六"];

function monthCells(year: number, month: number): (string | null)[] {
  const first = new Date(Date.UTC(year, month, 1));
  const days = new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
  const cells: (string | null)[] = [];
  for (let i = 0; i < first.getUTCDay(); i++) cells.push(null);
  for (let d = 1; d <= days; d++) cells.push(dayKey(year, month, d));
  return cells;
}

function CalendarMonth({
  year,
  month,
  selFrom,
  selTo,
  todayKey,
  onPick,
}: {
  year: number;
  month: number;
  selFrom: string | null;
  selTo: string | null;
  todayKey: string;
  onPick: (key: string) => void;
}) {
  const cells = monthCells(year, month);
  const inRange = (k: string) => !!selFrom && !!selTo && k > selFrom && k < selTo;
  return (
    <div>
      <div className="grid grid-cols-7 text-center text-[11px] text-[#5C6472] mb-1">
        {WEEKDAYS.map((w) => (
          <span key={w} className="py-1">
            {w}
          </span>
        ))}
      </div>
      <div className="grid grid-cols-7 gap-y-1">
        {cells.map((k, i) =>
          k === null ? (
            <span key={`blank-${i}`} />
          ) : (
            <button
              key={k}
              aria-label={k}
              disabled={k > todayKey}
              onClick={() => onPick(k)}
              className={`relative h-8 w-8 mx-auto text-[13px] transition-colors ${
                k > todayKey
                  ? "text-[#C6CAD4] cursor-not-allowed"
                  : selFrom === k || selTo === k
                    ? "bg-neutral-900 text-white font-semibold rounded-full"
                    : inRange(k)
                      ? "bg-[#F3F4F6] text-[#161A23]"
                      : "text-[#161A23] rounded-full hover:bg-black/5"
              }`}
            >
              {Number(k.slice(8, 10))}
              {k === todayKey && <span className="absolute top-0.5 right-0.5 w-1 h-1 rounded-full bg-neutral-900" />}
            </button>
          ),
        )}
      </div>
    </div>
  );
}

export interface RangePickerProps {
  from: Date;
  to: Date;
  preset: PresetKey;
  now?: Date;
  onApply: (range: { from: Date; to: Date; preset: PresetKey }) => void;
}

export default function RangePicker({ from, to, preset, now, onApply }: RangePickerProps) {
  const nowDate = useMemo(() => now ?? new Date(), [now]);
  const todayKey = useMemo(() => gmt8DayKey(nowDate.toISOString()), [nowDate]);
  const [open, setOpen] = useState(false);
  const [selFrom, setSelFrom] = useState<string | null>(null);
  const [selTo, setSelTo] = useState<string | null>(null);
  const [activePreset, setActivePreset] = useState<PresetKey>(preset);

  const anchor = useMemo(() => monthKeyFromInstant(from), [from]);
  const [viewYear, setViewYear] = useState(anchor.year);
  const [viewMonth, setViewMonth] = useState(anchor.month);

  useEffect(() => {
    if (!open) return;
    setSelFrom(gmt8DayKey(from.toISOString()));
    setSelTo(gmt8DayKey(to.toISOString()));
    setActivePreset(preset);
    const m = monthKeyFromInstant(from);
    setViewYear(m.year);
    setViewMonth(m.month);
  }, [open, from, to, preset]);

  const nextYear = viewMonth === 11 ? viewYear + 1 : viewYear;
  const nextMonth = viewMonth === 11 ? 0 : viewMonth + 1;

  const shift = (delta: number) => {
    const current = monthKeyFromInstant(nowDate);
    const currentIndex = current.year * 12 + current.month;
    const target = viewYear * 12 + viewMonth + delta;
    if (target > currentIndex) return; // 不翻到未来月份
    setViewYear(Math.floor(target / 12));
    setViewMonth(target % 12);
  };

  const close = () => setOpen(false);

  const applyPreset = (key: PresetKey) => {
    if (key === "custom") {
      setActivePreset("custom");
      return;
    }
    onApply({ ...rangeForPreset(key, nowDate), preset: key });
    setOpen(false);
  };

  const pickDay = (key: string) => {
    setActivePreset("custom");
    if (!selFrom || (selFrom && selTo)) {
      setSelFrom(key);
      setSelTo(null);
    } else if (key < selFrom) {
      setSelFrom(key);
      setSelTo(null);
    } else {
      setSelTo(key);
    }
  };

  const confirmCustom = () => {
    if (!selFrom || !selTo) return;
    const [start, end] = selFrom <= selTo ? [selFrom, selTo] : [selTo, selFrom];
    onApply({ from: dayKeyToStart(start), to: dayKeyToEnd(end), preset: "custom" });
    setOpen(false);
  };

  return (
    <div className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="glass-soft rounded-lg px-3 py-2 text-sm inline-flex items-center gap-2 focus:outline-none focus:ring-2 focus:ring-[#4F6BED]/20"
      >
        <CalendarDays size={14} className="text-[#5C6472]" />
        <span className="font-mono">{formatRangeLabel(from, to)}</span>
        <ChevronDown size={14} className="text-[#5C6472]" />
      </button>

      {open && (
        <>
          <div className="fixed inset-0 z-30" onClick={close} />
          <div className="absolute left-0 top-full mt-2 z-40 w-[720px] max-w-[calc(100vw-2rem)] rounded-lg bg-white border border-black/10 shadow-lg">
            <div className="grid grid-cols-[150px_1fr] divide-x divide-[#E5E7EB]">
              <div className="p-2">
                {PRESETS.map((p) => (
                  <button
                    key={p.key}
                    onClick={() => applyPreset(p.key)}
                    className={`w-full flex items-center justify-between rounded-md px-3 py-2 text-left text-[13px] transition-colors ${
                      activePreset === p.key
                        ? "bg-[#F3F4F6] text-[#161A23] font-semibold"
                        : "text-[#5C6472] hover:bg-black/5"
                    }`}
                  >
                    {p.label}
                    {activePreset === p.key && <Check size={14} className="text-[#4F6BED]" />}
                  </button>
                ))}
              </div>
              <div className="p-4">
                <div className="flex items-center justify-between mb-3">
                  <button onClick={() => shift(-1)} aria-label="上个月" className="p-1 rounded hover:bg-black/5">
                    <ChevronLeft size={16} />
                  </button>
                  <div className="flex gap-10 text-[13px] font-bold text-[#161A23]">
                    <span>
                      {viewYear}年{viewMonth + 1}月
                    </span>
                    <span>
                      {nextYear}年{nextMonth + 1}月
                    </span>
                  </div>
                  <button onClick={() => shift(1)} aria-label="下个月" className="p-1 rounded hover:bg-black/5">
                    <ChevronRight size={16} />
                  </button>
                </div>
                <div className="grid grid-cols-2 gap-5">
                  <CalendarMonth
                    year={viewYear}
                    month={viewMonth}
                    selFrom={selFrom}
                    selTo={selTo}
                    todayKey={todayKey}
                    onPick={pickDay}
                  />
                  <CalendarMonth
                    year={nextYear}
                    month={nextMonth}
                    selFrom={selFrom}
                    selTo={selTo}
                    todayKey={todayKey}
                    onPick={pickDay}
                  />
                </div>
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-[#E5E7EB] p-3">
              <button
                onClick={close}
                className="rounded-lg border border-black/10 bg-white px-4 py-2 text-sm font-semibold hover:bg-black/5"
              >
                取消
              </button>
              <button
                onClick={confirmCustom}
                disabled={!selFrom || !selTo}
                className="rounded-lg bg-neutral-900 px-4 py-2 text-sm font-semibold text-white hover:bg-neutral-800 disabled:opacity-40"
              >
                确定
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

// TokenHub 平台品牌标识：参考稿的几何 DEEPTROLS 字标，按项目主题色渲染。
// 无背景、无色块底：D 左侧断口、两个 E 为三根等长横杠（无竖边）、
// R 右下斜切缺口、O 圆润几何圆、S 流畅曲线，整体向右微斜。
// 颜色用项目主色：紫色 #8B6FE8 → 靛蓝 #4F6BED 渐变。

export interface BrandLogoProps {
  className?: string;
}

export default function BrandLogo({ className }: BrandLogoProps) {
  return (
    <svg
      viewBox="0 0 640 96"
      className={className ?? "w-[160px]"}
      role="img"
      aria-label="DEEPTROLS"
      xmlns="http://www.w3.org/2000/svg"
    >
      <defs>
        <linearGradient id="brand-grad" x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stopColor="#8B6FE8" />
          <stop offset="55%" stopColor="#6B63E8" />
          <stop offset="100%" stopColor="#4F6BED" />
        </linearGradient>
      </defs>
      <g transform="skewX(-5)" fill="url(#brand-grad)">
        {/* D：左侧竖边中间断开 */}
        <path d="M 35 12 L 55 12 Q 81 12 81 48 Q 81 84 55 84 L 35 84 L 35 64 L 23 64 L 23 32 L 35 32 Z" />

        {/* E（两个）：三根等长水平横杠，无竖边 */}
        <rect x="92" y="14" width="56" height="12" />
        <rect x="92" y="42" width="56" height="12" />
        <rect x="92" y="70" width="56" height="12" />
        <rect x="159" y="14" width="56" height="12" />
        <rect x="159" y="42" width="56" height="12" />
        <rect x="159" y="70" width="56" height="12" />

        {/* P：直角转折 */}
        <rect x="224" y="12" width="13" height="72" />
        <path d="M 237 12 L 259 12 Q 282 12 282 30 Q 282 48 259 48 L 237 48 Z" />

        {/* T：横平竖直 */}
        <rect x="291" y="12" width="58" height="13" />
        <rect x="313" y="25" width="13" height="59" />

        {/* R：右下角斜切缺口 */}
        <rect x="358" y="12" width="13" height="72" />
        <path d="M 371 12 L 393 12 Q 416 12 416 30 Q 416 48 393 48 L 371 48 Z" />
        <path d="M 371 48 L 393 48 L 389 66 L 378 84 L 362 84 Z" />

        {/* O：圆润几何圆形 */}
        <circle cx="454" cy="48" r="24" fill="none" stroke="url(#brand-grad)" strokeWidth="12" />

        {/* L：粗体几何 L */}
        <rect x="492" y="12" width="13" height="72" />
        <rect x="492" y="71" width="58" height="13" />

        {/* S：流畅曲线 */}
        <path
          d="M 608 20 C 616 8 574 4 568 17 C 563 29 603 33 605 48 C 607 66 572 75 566 61"
          fill="none"
          stroke="url(#brand-grad)"
          strokeWidth="12"
          strokeLinecap="butt"
        />
      </g>
    </svg>
  );
}

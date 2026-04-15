/**
 * OpenLoom dark theme — matches dashboard CSS variables exactly.
 * Use via tailwind classes (preferred) or direct import for StyleSheet.
 */

export const colors = {
  bg: {
    primary: "#0a0a0f",
    secondary: "#12121a",
    sidebar: "#0e0e16",
    card: "rgba(26, 26, 46, 0.7)",
    cardHover: "rgba(26, 26, 46, 0.9)",
    input: "rgba(255, 255, 255, 0.05)",
  },
  text: {
    primary: "#e4e4e7",
    secondary: "#a1a1aa",
    muted: "#71717a",
  },
  accent: {
    default: "#3b82f6",
    hover: "#60a5fa",
  },
  status: {
    success: "#00d97e",
    warning: "#f5c542",
    error: "#e63757",
  },
  border: {
    default: "rgba(255, 255, 255, 0.08)",
    hover: "rgba(255, 255, 255, 0.15)",
  },
} as const;

/** Tab bar icons — Unicode chars matching dashboard sidebar */
export const tabIcons = {
  overview: "\u25CB", // ○
  manifests: "\u2637", // ☷
  tasks: "\u23F0", // ⏰
  memories: "\u25A1", // □
  ideas: "\u2726", // ✦
  visceral: "\u2665", // ♥
  activity: "\u25B7", // ▷
  recall: "\u21A9", // ↩
  settings: "\u2699", // ⚙
} as const;

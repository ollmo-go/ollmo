// Central color tokens: every badge, accent, status and node-palette class
// lives here so components stop re-declaring their own copies. Tailwind
// needs static class names, so each variant is enumerated as a literal;
// components import the maps instead of writing color classes inline.

// Soft icon badges (icon chips on stat cards): light tint + mid hue.
export const ICON_BADGE: Record<string, string> = {
  teal: "bg-teal-50 text-teal-600 dark:bg-teal-950/50 dark:text-teal-400",
  green: "bg-green-50 text-green-600 dark:bg-green-950/50 dark:text-green-400",
  violet: "bg-violet-50 text-violet-600 dark:bg-violet-950/50 dark:text-violet-400",
  purple: "bg-purple-50 text-purple-600 dark:bg-purple-950/50 dark:text-purple-400",
  amber: "bg-amber-50 text-amber-600 dark:bg-amber-950/50 dark:text-amber-400",
  cyan: "bg-cyan-50 text-cyan-600 dark:bg-cyan-950/50 dark:text-cyan-400",
  orange: "bg-orange-50 text-orange-600 dark:bg-orange-950/50 dark:text-orange-400",
  rose: "bg-rose-50 text-rose-600 dark:bg-rose-950/50 dark:text-rose-400",
  indigo: "bg-indigo-50 text-indigo-600 dark:bg-indigo-950/50 dark:text-indigo-400",
};

// Bill record source badges: label key + classes. Shared by the analytics
// bills tab and the profile usage widget.
export const BILL_SOURCE: Record<string, { labelKey: string; cls: string }> = {
  chat: { labelKey: "bill.source_chat", cls: ICON_BADGE.teal },
  classifier: { labelKey: "bill.source_classifier", cls: ICON_BADGE.violet },
  intermediate: { labelKey: "bill.source_intermediate", cls: ICON_BADGE.amber },
  followups: { labelKey: "bill.source_followups", cls: ICON_BADGE.cyan },
};

// Solid status badges (100/700): team/admin roles, member/user statuses.
export const STATUS_BADGE: Record<"ok" | "warn" | "danger" | "info", string> = {
  ok: "bg-emerald-100 text-emerald-700",
  warn: "bg-amber-100 text-amber-700",
  danger: "bg-red-100 text-red-700",
  info: "bg-teal-100 text-teal-700",
};

// Soft status badges with dark-mode variants (analytics status chips).
export const STATUS_SOFT: Record<string, string> = {
  ok: "bg-green-100 text-green-700 dark:bg-green-950/50 dark:text-green-400",
  warn: "bg-amber-100 text-amber-700 dark:bg-amber-950/50 dark:text-amber-400",
  danger: "bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-400",
};

// Sidebar nav icon palette, keyed by the token names used in nav items.
export const ICON_COLOR: Record<string, string> = {
  teal: "text-teal-500 dark:text-teal-400",
  green: "text-green-500 dark:text-green-400",
  purple: "text-purple-500 dark:text-purple-400",
  orange: "text-orange-500 dark:text-orange-400",
  cyan: "text-cyan-500 dark:text-cyan-400",
  indigo: "text-indigo-500 dark:text-indigo-400",
  amber: "text-amber-500 dark:text-amber-400",
  rose: "text-rose-500 dark:text-rose-400",
};

// Avatar palette: hash-stable background per display name.
export const AVATAR_BG = [
  "bg-blue-500",
  "bg-emerald-500",
  "bg-violet-500",
  "bg-amber-500",
  "bg-rose-500",
  "bg-cyan-500",
  "bg-indigo-500",
  "bg-teal-500",
];

// Agent canvas node accents: hex for react-flow plus the icon class.
export const AGENT_NODE_ACCENTS: Record<string, { color: string; icon: string }> = {
  classifier: { color: "#8b5cf6", icon: "text-violet-500" },
  retrieval: { color: "#10b981", icon: "text-emerald-500" },
  condition: { color: "#f59e0b", icon: "text-amber-500" },
  llm: { color: "#0d9488", icon: "text-teal-500" },
  message: { color: "#06b6d4", icon: "text-cyan-500" },
};

// Retrieval-test drawer badges (solid pill + score chip styles).
export const BADGE_SOLID: Record<string, string> = {
  teal: "bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-400",
  green: "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400",
  purple: "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
  amber: "bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400",
};

export const BADGE_SOFT: Record<string, string> = {
  teal: "bg-teal-50 text-teal-600 dark:bg-teal-900/20 dark:text-teal-400",
  green: "bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-400",
  purple: "bg-purple-50 text-purple-600 dark:bg-purple-900/20 dark:text-purple-400",
  amber: "bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-400",
};

// Provider card status dots (settings page header).
export const STATUS_DOT: Record<"ok" | "warn" | "idle", string> = {
  ok: "bg-emerald-500",
  warn: "bg-amber-500",
  idle: "bg-muted-foreground/40",
};

// Quota/capacity bar color by usage percent (dashboard).
export function capacityBarColor(pct: number): string {
  if (pct >= 90) return "bg-red-500 dark:bg-red-400";
  if (pct >= 60) return "bg-amber-500 dark:bg-amber-400";
  return "bg-green-500 dark:bg-green-400";
}

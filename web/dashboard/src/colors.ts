// Colours used by the map and SVG charts (which cannot read CSS variables
// cheaply). Status colours are reserved for risk and exposure state and always
// appear next to a text label. Tuned for the dark control-room theme and the
// satellite basemap; they match the --lv-* / --st-* tokens in styles.css.

import type { ImpactStatus, RiskLevel } from "./types";

export const LEVEL_COLOR: Record<RiskLevel, string> = {
  low: "#3fb950",
  moderate: "#e3b341",
  high: "#f0883e",
  very_high: "#f85149",
};

export const STATUS_COLOR: Record<ImpactStatus, string> = {
  in_flood_path: "#f85149",
  at_risk: "#f0883e",
  outside: "#8193a9",
};

export const COLORS = {
  ink: "#1b2430",
  muted: "#8193a9",
  accent: "#56c5d0",
  outline: "#1f5fd1",
  outlineSat: "#ffe14d",
  outlineStack: "#1f5fd1",
  outlineStackSat: "#ffffff",
  glacier: "#bfe3f6",
  glacierLine: "#6cb4dc",
  corridorSevere: "#f2a0a0",
  corridorExpected: "#c0392b",
  floodPath: "#203a5c",
  floodPathSat: "#7fd3ff",
};

/** Chart ink on the dark panels. */
export const CHART = {
  line: "#e6edf6",
  accent: "#56c5d0",
  muted: "#8193a9",
  whisker: "#b4c2d4",
  surface: "#121f31",
};

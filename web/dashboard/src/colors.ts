// Colours used by the map and SVG charts (which cannot read CSS variables
// cheaply). Status colours are reserved for risk and exposure state and always
// appear next to a text label.

import type { ImpactStatus, RiskLevel } from "./types";

export const LEVEL_COLOR: Record<RiskLevel, string> = {
  low: "#0ca30c",
  moderate: "#e8a50c",
  high: "#ec7a3c",
  very_high: "#d03b3b",
};

export const STATUS_COLOR: Record<ImpactStatus, string> = {
  in_flood_path: "#d03b3b",
  at_risk: "#ec7a3c",
  outside: "#8a8f98",
};

export const COLORS = {
  ink: "#1b2430",
  muted: "#8a8f98",
  accent: "#2a63c4",
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

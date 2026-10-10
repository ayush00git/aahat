// Light or dark. The device decides (prefers-color-scheme) until the person
// picks one with the header button; the choice is then kept on this phone and
// set as data-theme on <html>, which styles.css keys on. index.html applies the
// saved choice before the first paint; keep the two in step.

import { useEffect, useState } from 'preact/hooks';
import { load, save } from './storage';

export type Theme = 'light' | 'dark';

/** The browser bar colour for each theme (the header background, --bg-2). */
const BAR: Record<Theme, string> = { light: '#ffffff', dark: '#0f1722' };

const mq = typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: dark)') : null;
const subs = new Set<() => void>();

function saved(): Theme | null {
  const v = load<Theme>('theme');
  return v === 'light' || v === 'dark' ? v : null;
}

let choice: Theme | null = saved();

export const currentTheme = (): Theme => choice ?? (mq?.matches ? 'dark' : 'light');

function apply(): void {
  const root = document.documentElement;
  if (choice) root.setAttribute('data-theme', choice);
  else root.removeAttribute('data-theme');
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', BAR[currentTheme()]);
  subs.forEach((f) => f());
}

export function setTheme(t: Theme): void {
  choice = t;
  save('theme', t);
  apply();
}

/** A CSS custom property of the active theme, for colours used from JS (the map). */
export function cssVar(name: string, fallback: string): string {
  try {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
  } catch {
    return fallback;
  }
}

export function useTheme(): Theme {
  const [theme, set] = useState<Theme>(currentTheme);
  useEffect(() => {
    const on = () => set(currentTheme());
    subs.add(on);
    on();
    return () => {
      subs.delete(on);
    };
  }, []);
  return theme;
}

// The device switches (sunset, battery saver) while the app is open.
mq?.addEventListener?.('change', apply);
apply();

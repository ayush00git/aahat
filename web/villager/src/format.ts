// Formatting only: API numbers are shown, never derived.

import type { Lang } from './i18n';
import type { Place, Threat } from './api';

/**
 * Whole minutes, rounded down — the same rule the API's SMS / voice / push
 * messages use, so the app and the message never disagree.
 */
export const wholeMinutes = (m: number) => Math.floor(m);

const locale = (lang: Lang) => (lang === 'hi' ? 'hi-IN' : 'en-IN');

/** A number with at most `digits` decimals, e.g. 4.7 or 54. */
export function num(n: number, lang: Lang, digits = 1): string {
  return n.toLocaleString(locale(lang), { maximumFractionDigits: digits });
}

/**
 * Clock options for the language. hi-IN would otherwise print a Latin
 * "am/pm" inside a Hindi sentence, so Hindi uses the spoken day period
 * ("दोपहर 1:05"); browsers that don't know the option fall back to am/pm.
 */
const clock = (lang: Lang): Intl.DateTimeFormatOptions =>
  lang === 'hi'
    ? { hour: 'numeric', minute: '2-digit', hour12: true, dayPeriod: 'short' }
    : { hour: 'numeric', minute: '2-digit', hour12: true };

export function dateTime(ms: number, lang: Lang): string {
  return new Date(ms).toLocaleString(locale(lang), { day: 'numeric', month: 'short', ...clock(lang) });
}

export function time(ms: number, lang: Lang): string {
  return new Date(ms).toLocaleTimeString(locale(lang), clock(lang));
}

type Named = Pick<Place, 'name' | 'name_hi'> & { osm?: string };

/** placeName prefers the name in the chosen language, falling back to the other. */
export function placeName(p: Named, lang: Lang): string {
  const first = lang === 'hi' ? p.name_hi : p.name;
  const second = lang === 'hi' ? p.name : p.name_hi;
  return first || second || p.osm || '—';
}

/** The other-language name, when it differs (shown small under the main one). */
export function altName(p: Named, lang: Lang): string | null {
  const alt = lang === 'hi' ? p.name : p.name_hi;
  return alt && alt !== placeName(p, lang) ? alt : null;
}

export const lakeName = (t: Pick<Threat, 'lake_name' | 'lake_name_hi'>, lang: Lang) =>
  (lang === 'hi' ? t.lake_name_hi : t.lake_name) || t.lake_name || t.lake_name_hi;

const HOME_STATE = 'himachal pradesh';

/**
 * "Hamirpur district" (plus the state when it is not Himachal Pradesh), so
 * two villages of the same name can be told apart. Null when the API gave neither.
 */
export function adminLine(
  p: { district?: string | null; state?: string | null },
  districtText: (d: string) => string,
): string | null {
  const parts: string[] = [];
  if (p.district) parts.push(districtText(p.district));
  if (p.state && p.state.trim().toLowerCase() !== HOME_STATE) parts.push(p.state);
  return parts.length ? parts.join(', ') : null;
}

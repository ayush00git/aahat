import { useI18n, type Lang } from '../i18n';
import { href } from '../router';
import { setTheme, useTheme } from '../theme';
import { IconMoon, IconSun } from './icons';

/** The app mark (same as the officials' dashboard): a peak above a glacial lake. */
export function Mark({ size = 28 }: { size?: number }) {
  return (
    <svg class="mark" width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" focusable="false">
      <rect class="mark-bg" width="32" height="32" rx="7" />
      <path class="mark-peak" d="M5 21.5 12.5 9l4 6.2 2.7-3.7 7.8 10Z" />
      <path class="mark-wave" d="M8 25.5q8-3.4 16 0" stroke-width="2.4" stroke-linecap="round" />
    </svg>
  );
}

export function Header() {
  const { t, lang } = useI18n();
  return (
    <header class="header">
      <a class="brand" href={href.home()} aria-label={`${t.homeLabel} — ${t.tagline}`}>
        <Mark />
        <span class="brand-name">
          <span lang="hi">आहट</span>
          <span class={lang === 'hi' ? 'brand-alt brand-alt-wide' : 'brand-alt'} lang="en">
            Aahat
          </span>
        </span>
      </a>
      <p class="header-tag">{t.tagline}</p>
      <div class="header-actions">
        <LangToggle />
        <ThemeToggle />
        <a class="officials" href="/officials/" title={t.officialsLabel} aria-label={t.officialsLabel}>
          {t.officials}
        </a>
      </div>
    </header>
  );
}

/** Shows where a tap leads: the moon in the light theme, the sun in the dark one. */
function ThemeToggle() {
  const { t } = useI18n();
  const theme = useTheme();
  const next = theme === 'dark' ? 'light' : 'dark';
  const label = next === 'dark' ? t.themeToDark : t.themeToLight;
  return (
    <button type="button" class="theme-btn" title={label} aria-label={label} onClick={() => setTheme(next)}>
      {next === 'dark' ? <IconMoon size={20} /> : <IconSun size={20} />}
    </button>
  );
}

function LangToggle() {
  const { lang, setLang, t } = useI18n();
  const opt = (l: Lang, label: string) => (
    <button type="button" class="lang-opt" lang={l} aria-pressed={lang === l} onClick={() => setLang(l)}>
      {label}
    </button>
  );
  return (
    <div class="lang-toggle" role="group" aria-label={t.langSwitch}>
      {opt('hi', 'हिंदी')}
      {opt('en', 'English')}
    </div>
  );
}

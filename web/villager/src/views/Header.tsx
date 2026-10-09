import { useI18n, type Lang } from '../i18n';
import { href } from '../router';

/** The app mark: a peak above a glacial lake, with the ripple of an आहट. */
export function Mark({ size = 32 }: { size?: number }) {
  return (
    <svg class="mark" width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" focusable="false">
      <rect width="32" height="32" rx="8" fill="#173f73" />
      <path d="M5 21.5 12.5 9l4 6.2 2.7-3.7 7.8 10Z" fill="#fff" />
      <path d="M12.5 9 14.6 12.3 13.3 13.5 12.5 12.4 11.4 13.6 10.6 12.7Z" fill="#bcd3ee" />
      <path d="M8 25.5q8-3.4 16 0" fill="none" stroke="#5fd4cf" stroke-width="2.2" stroke-linecap="round" />
    </svg>
  );
}

export function Header({ home }: { home: boolean }) {
  const { t } = useI18n();
  if (home) {
    // The home screen's hero carries the name; the bar keeps only the language switch.
    return (
      <header class="header header-home">
        <LangToggle />
      </header>
    );
  }
  return (
    <header class="header">
      <a class="brand" href={href.home()} aria-label={`${t.homeLabel} — ${t.tagline}`}>
        <Mark />
        <span class="brand-name" lang="hi">
          आहट
        </span>
      </a>
      <LangToggle />
    </header>
  );
}

function LangToggle() {
  const { lang, setLang, t } = useI18n();
  const opt = (l: Lang, label: string) => (
    <button
      type="button"
      class="lang-opt"
      lang={l}
      aria-pressed={lang === l}
      onClick={() => setLang(l)}
    >
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

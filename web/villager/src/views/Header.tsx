import { useI18n, type Lang } from '../i18n';
import { href } from '../router';

export function Header({ home }: { home: boolean }) {
  const { t } = useI18n();
  return (
    <header class={home ? 'header header-home' : 'header'}>
      <div class="header-row">
        <a class="brand" href={href.home()} aria-label={`${t.appName} — ${t.tagline}`}>
          <img src="./favicon.svg" alt="" width={home ? 44 : 32} height={home ? 44 : 32} />
          <span class="brand-name" lang="hi">
            आहट
          </span>
        </a>
        <LangToggle />
      </div>
      {home && <p class="tagline">{t.tagline}</p>}
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

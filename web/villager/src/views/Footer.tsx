import { useI18n } from '../i18n';
import { CREDITS } from '../content/safety';

export function Footer() {
  const { t, lang } = useI18n();
  return (
    <footer class="footer">
      <p class="footer-note">{t.footerNote}</p>
      <p class="footer-head">{t.sources}</p>
      <ul class="credits">
        {CREDITS[lang].map((c) => (
          <li key={c}>{c}</li>
        ))}
      </ul>
    </footer>
  );
}

import { useI18n } from '../i18n';
import { shareNative, whatsappHref } from '../share';
import { IconWhatsApp } from './icons';

/**
 * "Send on WhatsApp". On phones with a share sheet it opens that (same text
 * and link); elsewhere, or if the sheet fails, it opens a wa.me link.
 */
export function ShareButton({ text, link, invert }: { text: string; link: string; invert?: boolean }) {
  const { t } = useI18n();
  const wa = whatsappHref(text, link);
  const onClick = async (e: MouseEvent) => {
    if (typeof navigator.share !== 'function') return; // let the link open wa.me
    e.preventDefault();
    if (await shareNative(text, link)) return;
    // (window.open returns null with 'noopener', so clear opener by hand.)
    const w = window.open(wa, '_blank');
    if (w) w.opener = null;
    else location.href = wa;
  };
  return (
    <a class={`btn btn-share${invert ? ' btn-share-invert' : ''}`} href={wa} target="_blank" rel="noopener noreferrer" onClick={onClick}>
      <IconWhatsApp size={24} class="share-icon" />
      {t.shareWhatsApp}
    </a>
  );
}

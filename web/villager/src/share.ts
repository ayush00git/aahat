// Sharing a result on WhatsApp. The message text is built only from API
// values (names, whole minutes rounded down) by the strings in i18n.ts.

/** The app's own link to a place's result page. */
export function placeLink(osm: string): string {
  return `${location.origin}${location.pathname}#/p/${osm}`;
}

/** The wa.me link that opens WhatsApp with `text` and `link` filled in. */
export function whatsappHref(text: string, link: string): string {
  return `https://wa.me/?text=${encodeURIComponent(`${text} ${link}`)}`;
}

/**
 * share opens the phone's share sheet (where WhatsApp is usually one tap
 * away). Returns false when that isn't available, so the caller falls back
 * to the wa.me link.
 */
export async function shareNative(text: string, link: string): Promise<boolean> {
  if (typeof navigator.share !== 'function') return false;
  try {
    await navigator.share({ text, url: link });
  } catch (err) {
    // The person closed the share sheet: done, don't open WhatsApp anyway.
    if (err instanceof DOMException && err.name === 'AbortError') return true;
    return false;
  }
  return true;
}

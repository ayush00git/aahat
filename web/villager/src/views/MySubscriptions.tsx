import { useState } from 'preact/hooks';
import { ApiError, deleteSubscription } from '../api';
import { useI18n } from '../i18n';
import { getSubscriptions, removeSubscription, type SavedSubscription } from '../storage';
import { unsubscribePush } from '../push';

/** Warnings this phone signed up for at one place, each with a stop button. */
export function MySubscriptions({ osm }: { osm: string }) {
  const { t } = useI18n();
  const [subs, setSubs] = useState(() => getSubscriptions().filter((s) => s.place_osm === osm));
  const [message, setMessage] = useState<string | null>(null);

  if (subs.length === 0 && !message) return null;
  return (
    <section class="card card-info stack-sm" aria-live="polite">
      {subs.length > 0 && <p class="card-title-sm">{t.subscribedHere}</p>}
      {subs.map((s) => (
        <SubRow
          key={s.id}
          sub={s}
          onRemoved={() => {
            setSubs((xs) => xs.filter((x) => x.id !== s.id));
            setMessage(t.unsubscribed);
          }}
        />
      ))}
      {message && <p>{message}</p>}
    </section>
  );
}

export function SubRow({ sub, onRemoved }: { sub: SavedSubscription; onRemoved: () => void }) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const channel =
    sub.channel === 'webpush' ? t.channel_webpush : sub.channel === 'voice' ? t.channel_voice : t.channel_sms;

  const stop = async () => {
    setBusy(true);
    setError(null);
    try {
      await deleteSubscription(sub.id);
    } catch (err) {
      // Already gone on the server is fine; anything else, keep it and say so.
      if (!(err instanceof ApiError && err.status === 404)) {
        setError(t.loadError);
        setBusy(false);
        return;
      }
    }
    removeSubscription(sub.id);
    if (sub.channel === 'webpush' && !getSubscriptions().some((s) => s.channel === 'webpush')) {
      await unsubscribePush().catch(() => {});
    }
    onRemoved();
  };

  return (
    <div class="sub-row">
      <p>
        <strong>{channel}</strong>
        {sub.phone && <span class="mono"> · {sub.phone}</span>}
        <br />
        <span class="small muted">
          {t.subId}: <span class="mono">{sub.id}</span>
        </span>
      </p>
      <button type="button" class="btn btn-danger-outline" onClick={stop} disabled={busy}>
        {busy ? t.unsubscribing : t.unsubscribe}
      </button>
      {error && (
        <p class="error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

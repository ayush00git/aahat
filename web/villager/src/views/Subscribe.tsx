import { useEffect, useState } from 'preact/hooks';
import { ApiError, createSubscription, pushPublicKey, type Channel, type SubscribeRequest, type Threats } from '../api';
import { useI18n } from '../i18n';
import { placeName } from '../format';
import { href } from '../router';
import { addSubscription, type SavedSubscription } from '../storage';
import { PushDeniedError, pushSupported, subscribePush } from '../push';
import { usePlace } from '../usePlace';
import { BackLink, PlaceStatus } from './Place';
import { SubRow } from './MySubscriptions';

// The API cannot place voice calls to India yet; the option is shown as
// "coming soon" so people know it is planned. Flip this when it works.
const VOICE_AVAILABLE = false;

/** normalizePhone turns "98123 45678", "098123-45678" or "+91 9812345678" into E.164, or null. */
export function normalizePhone(input: string): string | null {
  let s = input.replace(/[\s\-().]/g, '');
  if (/^0\d{10}$/.test(s)) s = s.slice(1);
  if (/^\d{10}$/.test(s)) s = '+91' + s;
  if (/^91\d{10}$/.test(s)) s = '+' + s;
  if (s.startsWith('+91')) return /^\+91[6-9]\d{9}$/.test(s) ? s : null;
  return /^\+[1-9]\d{7,14}$/.test(s) ? s : null;
}

export function Subscribe({ osm }: { osm: string }) {
  const state = usePlace(osm);
  if (state.kind !== 'ok') return <PlaceStatus state={state} />;
  return <SubscribeForm osm={osm} place={state.data} />;
}

function SubscribeForm({ osm, place }: { osm: string; place: Threats }) {
  const { t, lang } = useI18n();
  const shown = placeName(place, lang);

  const [phone, setPhone] = useState('+91 ');
  const [channel, setChannel] = useState<Channel>('sms');
  const [pushKey, setPushKey] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [phoneError, setPhoneError] = useState(false);
  const [done, setDone] = useState<SavedSubscription | null>(null);
  const [stopped, setStopped] = useState(false);

  useEffect(() => {
    if (!pushSupported()) return;
    let live = true;
    pushPublicKey().then((k) => live && setPushKey(k));
    return () => {
      live = false;
    };
  }, []);

  const needsPhone = channel !== 'webpush';

  const submit = async (e: Event) => {
    e.preventDefault();
    setError(null);
    const e164 = normalizePhone(phone);
    if (needsPhone && !e164) {
      setPhoneError(true);
      document.getElementById('phone')?.focus();
      return;
    }
    setPhoneError(false);
    setBusy(true);
    try {
      const req: SubscribeRequest = { place_osm: osm, place_name: shown, lang, channel };
      if (needsPhone) req.phone = e164!;
      if (channel === 'webpush') req.push_subscription = await subscribePush(pushKey!);
      const sub = await createSubscription(req);
      const saved: SavedSubscription = {
        id: sub.id,
        place_osm: sub.place_osm,
        place_name: sub.place_name,
        channel: sub.channel,
        phone: sub.phone ?? '',
        created_at: sub.created_at,
      };
      addSubscription(saved);
      setDone(saved);
    } catch (err) {
      if (err instanceof PushDeniedError) setError(t.pushDenied);
      else if (err instanceof ApiError) setError(`${t.subError}: ${err.message}`);
      else if (channel === 'webpush' && navigator.onLine) setError(t.pushFailed);
      else setError(t.loadError);
    } finally {
      setBusy(false);
    }
  };

  if (done) {
    return (
      <div class="stack">
        <BackLink to={href.place(osm)} />
        <section class="card card-safe stack-sm" aria-live="polite">
          <h1 class="card-title">
            <span class="status-icon" aria-hidden="true">
              ✓
            </span>
            {stopped ? t.unsubscribed : t.success}
          </h1>
          {!stopped && (
            <>
              <p>{t.successBody(shown)}</p>
              <SubRow sub={done} onRemoved={() => setStopped(true)} />
            </>
          )}
        </section>
        <a class="btn btn-primary" href={href.place(osm)}>
          {t.done}
        </a>
      </div>
    );
  }

  const channels: { id: Channel; label: string; help?: string; disabled?: boolean }[] = [
    { id: 'sms', label: t.channel_sms },
    { id: 'voice', label: t.channel_voice, help: VOICE_AVAILABLE ? undefined : t.channelSoon, disabled: !VOICE_AVAILABLE },
  ];
  if (pushKey) channels.push({ id: 'webpush', label: t.channel_webpush, help: t.channelPushHelp });

  return (
    <div class="stack">
      <BackLink to={href.place(osm)} />
      <h1 class="page-title">{t.subscribeCta}</h1>
      <p>{t.subscribeIntro(shown)}</p>

      <form class="stack" onSubmit={submit} noValidate>
        <fieldset class="fieldset">
          <legend class="field-label">{t.channelLabel}</legend>
          <div class="choices">
            {channels.map((c) => (
              <label key={c.id} class={`choice${c.disabled ? ' choice-disabled' : ''}`}>
                <input
                  type="radio"
                  name="channel"
                  value={c.id}
                  checked={channel === c.id}
                  disabled={c.disabled}
                  onChange={() => setChannel(c.id)}
                />
                <span>
                  <span class="choice-label">{c.label}</span>
                  {c.help && <span class="choice-help">{c.help}</span>}
                </span>
              </label>
            ))}
          </div>
        </fieldset>

        {needsPhone && (
          <div class="field">
            <label for="phone" class="field-label">
              {t.phoneLabel}
            </label>
            <input
              id="phone"
              class="text-input mono"
              type="tel"
              inputMode="tel"
              autocomplete="tel"
              value={phone}
              onInput={(e) => setPhone((e.target as HTMLInputElement).value)}
              aria-invalid={phoneError}
              aria-describedby="phone-help"
            />
            <p id="phone-help" class={phoneError ? 'error' : 'hint'} role={phoneError ? 'alert' : undefined}>
              {phoneError ? t.phoneInvalid : t.phoneHelp}
            </p>
          </div>
        )}

        <p class="small muted">
          {t.messageLang} {needsPhone && t.consent}
        </p>

        {error && (
          <p class="error card card-neutral" role="alert">
            {error}
          </p>
        )}

        <button type="submit" class="btn btn-primary" disabled={busy}>
          {busy ? t.submitting : t.submit}
        </button>
      </form>
    </div>
  );
}

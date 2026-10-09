import { useEffect, useRef, useState } from 'preact/hooks';
import { url } from '../api';
import { useI18n } from '../i18n';
import { num, time, wholeMinutes } from '../format';
import { href } from '../router';
import { ACTIONS } from '../content/safety';
import { EmergencyNumbers } from './Actions';

/** The push payload, as the service worker hands it over. */
export interface AlertPayload {
  title?: string;
  body?: string;
  lang?: string;
  audio_url?: string;
  lake_id?: string;
  place_osm?: string;
  arrival_min_fast?: number | null;
  event_id?: string;
  received_at?: number;
}

/** alertHash builds the #/alert route for a payload (same keys as sw/sw.js). */
export function alertHash(p: AlertPayload): string {
  const q = new URLSearchParams();
  const set = (k: string, v: unknown) => {
    if (v !== undefined && v !== null && v !== '') q.set(k, String(v));
  };
  set('title', p.title);
  set('body', p.body);
  set('lang', p.lang);
  set('audio', p.audio_url);
  set('lake', p.lake_id);
  set('place', p.place_osm);
  set('min', p.arrival_min_fast);
  set('event', p.event_id);
  set('at', p.received_at);
  return `#/alert?${q}`;
}

/** Only play audio from our API (a path) or an https URL. */
function audioSrc(raw: string | null): string | null {
  if (!raw) return null;
  if (raw.startsWith('/')) return url(raw);
  return /^https:\/\//.test(raw) ? raw : null;
}

export function Alert({ params }: { params: URLSearchParams }) {
  const { t, lang } = useI18n();
  const title = params.get('title') || t.alertHeading;
  const body = params.get('body') || '';
  const msgLang = params.get('lang') === 'en' ? 'en' : 'hi';
  const minRaw = params.get('min');
  const minutes = minRaw !== null && minRaw !== '' && Number.isFinite(Number(minRaw)) ? Number(minRaw) : null;
  const at = Number(params.get('at')) || null;
  const place = params.get('place');
  const src = audioSrc(params.get('audio'));

  const audio = useRef<HTMLAudioElement>(null);
  const [playing, setPlaying] = useState(false);
  const [audioError, setAudioError] = useState(false);
  const heading = useRef<HTMLHeadingElement>(null);

  useEffect(() => {
    heading.current?.focus();
    document.body.classList.add('alert-mode');
    // Try to start the voice message; browsers may block this until a tap.
    audio.current?.play().catch(() => {});
    return () => document.body.classList.remove('alert-mode');
  }, []);

  const toggle = () => {
    const a = audio.current;
    if (!a) return;
    if (a.paused) a.play().catch(() => setAudioError(true));
    else a.pause();
  };

  return (
    <div class="alert" role="alertdialog" aria-labelledby="alert-h" aria-describedby="alert-body">
      <p class="alert-kicker">
        <span aria-hidden="true">⚠</span> {t.alertHeading}
      </p>
      <h1 id="alert-h" class="alert-title" tabIndex={-1} ref={heading} lang={msgLang}>
        {title}
      </h1>

      {minutes !== null && (
        <div class="alert-count" aria-live="assertive">
          <p class="alert-count-label">{t.alertArrival}</p>
          <p class="alert-count-num">
            ~{num(wholeMinutes(minutes), lang, 0)} <span class="alert-count-unit">{t.minutes}</span>
          </p>
        </div>
      )}

      <p class="alert-go">{t.alertGo}</p>

      {body && (
        <p id="alert-body" class="alert-body" lang={msgLang}>
          {body}
        </p>
      )}

      {src && (
        <div class="alert-audio">
          <audio
            ref={audio}
            src={src}
            preload="auto"
            onPlay={() => setPlaying(true)}
            onPause={() => setPlaying(false)}
            onEnded={() => setPlaying(false)}
            onError={() => setAudioError(true)}
          />
          <button type="button" class="btn btn-alert" onClick={toggle} aria-pressed={playing}>
            <span aria-hidden="true">{playing ? '❚❚' : '▶'}</span> {playing ? t.alertPause : t.alertPlay}
          </button>
          {audioError && <p class="alert-small">{t.alertAudioError}</p>}
        </div>
      )}

      <ol class="alert-actions">
        {ACTIONS[lang].slice(0, 3).map((a) => (
          <li key={a}>{a}</li>
        ))}
      </ol>

      <EmergencyNumbers invert />

      {at && <p class="alert-small">{t.alertReceived(time(at, lang))}</p>}

      <a class="btn btn-alert-outline" href={place ? href.place(place) : href.home()}>
        {t.alertClose}
      </a>
    </div>
  );
}

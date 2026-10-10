import { useEffect, useRef, useState } from 'preact/hooks';
import { ApiError, ask, url, type AskAnswer } from '../api';
import { useI18n } from '../i18n';
import { IconPause, IconPlay, IconSend } from './icons';

const MAX_LEN = 500;

type State =
  | { kind: 'idle' }
  | { kind: 'loading'; question: string }
  | { kind: 'done'; question: string; answer: AskAnswer }
  | { kind: 'error'; message: string };

/** Only play audio from our API (a path) or an https URL (as on the alert screen). */
function audioSrc(raw: string | null | undefined): string | null {
  if (!raw) return null;
  if (raw.startsWith('/')) return url(raw);
  return /^https:\/\//.test(raw) ? raw : null;
}

/**
 * "Ask a question": one question to the Aahat assistant (POST /ask), one
 * answer shown as plain text, with the spoken version when the API made one.
 * Only the latest answer is kept. A failure shows one quiet line here and
 * never touches the rest of the page.
 */
export function Ask({ osm }: { osm?: string }) {
  const { t, lang } = useI18n();
  const [q, setQ] = useState('');
  const [state, setState] = useState<State>({ kind: 'idle' });
  const seq = useRef(0);
  useEffect(() => () => void seq.current++, []); // ignore an answer that arrives after leaving

  const send = async (text: string) => {
    const question = text.trim().slice(0, MAX_LEN);
    if (!question || state.kind === 'loading') return;
    const mine = ++seq.current;
    setState({ kind: 'loading', question });
    try {
      const answer = await ask({ question, lang, place_osm: osm });
      if (mine !== seq.current) return;
      if (!answer || typeof answer.answer !== 'string' || !answer.answer.trim()) throw new Error('empty answer');
      setState({ kind: 'done', question, answer });
      setQ('');
    } catch (err) {
      if (mine !== seq.current) return;
      setState({
        kind: 'error',
        message: err instanceof ApiError && err.status === 429 ? t.askRateLimit : t.askUnavailable,
      });
    }
  };

  // "Which lake threatens my village?" only makes sense with a village on screen.
  const examples = osm ? t.askExamples : t.askExamples.slice(1);
  const busy = state.kind === 'loading';

  return (
    <section class="card ask" aria-labelledby="ask-h">
      <h2 id="ask-h" class="ask-title">
        <label for="ask-q">{t.askTitle}</label>
      </h2>
      <form
        class="ask-form"
        onSubmit={(e) => {
          e.preventDefault();
          void send(q);
        }}
      >
        <input
          id="ask-q"
          class="text-input"
          type="text"
          enterkeyhint="send"
          autocomplete="off"
          maxLength={MAX_LEN}
          placeholder={t.askPlaceholder}
          value={q}
          onInput={(e) => setQ((e.target as HTMLInputElement).value)}
        />
        <button type="submit" class="btn btn-primary ask-send" disabled={busy || !q.trim()} aria-label={t.askSend}>
          <IconSend size={18} />
        </button>
      </form>

      <ul class="ask-examples" aria-label={t.askExamplesLabel}>
        {examples.map((ex) => (
          <li key={ex}>
            <button type="button" class="chip" disabled={busy} onClick={() => void send(ex)}>
              {ex}
            </button>
          </li>
        ))}
      </ul>

      <div aria-live="polite">
        {state.kind === 'loading' && (
          <div class="ask-result">
            <p class="ask-question">{state.question}</p>
            <p class="ask-loading">
              <span class="ask-dots" aria-hidden="true">
                <span />
                <span />
                <span />
              </span>
              {t.askLoading}
            </p>
          </div>
        )}
        {state.kind === 'error' && <p class="ask-error">{state.message}</p>}
        {state.kind === 'done' && (
          <div class="ask-result">
            <p class="ask-question">{state.question}</p>
            <p class="ask-answer" lang={state.answer.lang === 'en' ? 'en' : 'hi'}>
              {state.answer.answer}
            </p>
            <AnswerAudio key={state.answer.audio_url ?? ''} src={audioSrc(state.answer.audio_url)} />
            <p class="ask-note">{t.askNote}</p>
          </div>
        )}
      </div>
    </section>
  );
}

function AnswerAudio({ src }: { src: string | null }) {
  const { t } = useI18n();
  const audio = useRef<HTMLAudioElement>(null);
  const [playing, setPlaying] = useState(false);
  const [failed, setFailed] = useState(false);
  if (!src || failed) return null;
  const toggle = () => {
    const a = audio.current;
    if (!a) return;
    if (a.paused) a.play().catch(() => setFailed(true));
    else a.pause();
  };
  return (
    <div>
      <audio
        ref={audio}
        src={src}
        preload="none"
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        onEnded={() => setPlaying(false)}
        onError={() => setFailed(true)}
      />
      <button type="button" class="btn btn-secondary ask-play" onClick={toggle} aria-pressed={playing}>
        {playing ? <IconPause size={16} /> : <IconPlay size={16} />}
        {playing ? t.alertPause : t.askListen}
      </button>
    </div>
  );
}

import { useEffect, useState } from 'preact/hooks';
import { searchPlaces, type Place } from '../api';
import { useI18n } from '../i18n';
import { altName, placeName } from '../format';
import { href } from '../router';
import { getLastPlace } from '../storage';
import { rememberCoords } from '../coords';

const DEBOUNCE_MS = 350;

type State =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'done'; results: Place[] }
  | { kind: 'error' };

export function Home() {
  const { t, lang } = useI18n();
  const [q, setQ] = useState('');
  const [state, setState] = useState<State>({ kind: 'idle' });
  const [attempt, setAttempt] = useState(0);
  const last = getLastPlace();

  useEffect(() => {
    const query = q.trim();
    if (query.length < 2) {
      setState({ kind: 'idle' });
      return;
    }
    const ctrl = new AbortController();
    const timer = setTimeout(async () => {
      setState({ kind: 'loading' });
      try {
        const results = await searchPlaces(query, ctrl.signal);
        rememberCoords(results);
        setState({ kind: 'done', results });
      } catch {
        if (!ctrl.signal.aborted) setState({ kind: 'error' });
      }
    }, DEBOUNCE_MS);
    return () => {
      clearTimeout(timer);
      ctrl.abort();
    };
  }, [q, attempt]);

  return (
    <div class="stack">
      <p class="lead">{t.intro}</p>

      <form class="search" role="search" onSubmit={(e) => e.preventDefault()}>
        <label for="q" class="search-label">
          {t.searchLabel}
        </label>
        <input
          id="q"
          class="search-input"
          type="search"
          inputMode="search"
          autocomplete="off"
          autocapitalize="words"
          spellcheck={false}
          placeholder={t.searchPlaceholder}
          value={q}
          onInput={(e) => setQ((e.target as HTMLInputElement).value)}
          aria-describedby="q-hint"
          aria-controls="results"
        />
        <p id="q-hint" class="hint">
          {t.searchHint}
        </p>
      </form>

      <div id="results" aria-live="polite">
        {state.kind === 'loading' && <p class="muted">{t.searching}</p>}
        {state.kind === 'error' && (
          <div class="card card-neutral">
            <p>{t.searchError}</p>
            <button type="button" class="btn btn-secondary" onClick={() => setAttempt((n) => n + 1)}>
              {t.retry}
            </button>
          </div>
        )}
        {state.kind === 'done' && state.results.length === 0 && <p class="card card-neutral">{t.noResults}</p>}
        {state.kind === 'done' && state.results.length > 0 && (
          <ul class="results" aria-label={t.resultsLabel}>
            {state.results.map((p) => (
              <li key={p.osm}>
                <a class="result" href={href.place(p.osm)}>
                  <span class="result-name">{placeName(p, lang)}</span>
                  {altName(p, lang) && <span class="result-alt">{altName(p, lang)}</span>}
                  <span class="chev" aria-hidden="true">
                    ›
                  </span>
                </a>
              </li>
            ))}
          </ul>
        )}
      </div>

      {last && state.kind === 'idle' && (
        <section class="stack-sm" aria-labelledby="last-h">
          <h2 id="last-h" class="section-label">
            {t.lastViewed}
          </h2>
          <a class="result" href={href.place(last.osm)}>
            <span class="result-name">{placeName(last, lang)}</span>
            <span class="chev" aria-hidden="true">
              ›
            </span>
          </a>
        </section>
      )}

      <InstallButton />
    </div>
  );
}

interface BeforeInstallPromptEvent extends Event {
  prompt(): Promise<void>;
}

let deferredPrompt: BeforeInstallPromptEvent | null = null;
const listeners = new Set<() => void>();
window.addEventListener('beforeinstallprompt', (e) => {
  e.preventDefault();
  deferredPrompt = e as BeforeInstallPromptEvent;
  listeners.forEach((f) => f());
});

function InstallButton() {
  const { t } = useI18n();
  const [, force] = useState(0);
  useEffect(() => {
    const f = () => force((n) => n + 1);
    listeners.add(f);
    return () => void listeners.delete(f);
  }, []);
  if (!deferredPrompt) return null;
  return (
    <button
      type="button"
      class="btn btn-secondary"
      onClick={async () => {
        const p = deferredPrompt;
        deferredPrompt = null;
        force((n) => n + 1);
        await p?.prompt();
      }}
    >
      {t.install}
    </button>
  );
}

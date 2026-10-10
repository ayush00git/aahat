import { useEffect, useState } from 'preact/hooks';
import { searchPlaces, type Place } from '../api';
import { STRINGS, useI18n } from '../i18n';
import { adminLine, altName, placeName } from '../format';
import { href } from '../router';
import { getLastPlace } from '../storage';
import { rememberCoords } from '../coords';
import { IconChevron, IconClock, IconPin, IconSearch } from './icons';

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
      <Hero />

      <form class="search" role="search" onSubmit={(e) => e.preventDefault()}>
        <label for="q" class="search-label">
          {t.searchLabel}
        </label>
        <div class="search-box">
          <IconSearch class="search-icon" />
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
          {state.kind === 'loading' && <span class="search-spinner" aria-hidden="true" />}
        </div>
        <p id="q-hint" class="hint">
          {t.searchHint}
        </p>
      </form>
      {state.kind === 'idle' && <p class="lead">{t.intro}</p>}

      <div id="results" aria-live="polite">
        {state.kind === 'loading' && <p class="sr-only">{t.searching}</p>}
        {state.kind === 'error' && (
          <div class="note note-neutral stack-sm">
            <p>{t.searchError}</p>
            <button type="button" class="btn btn-secondary" onClick={() => setAttempt((n) => n + 1)}>
              {t.retry}
            </button>
          </div>
        )}
        {state.kind === 'done' && state.results.length === 0 && <p class="note note-neutral">{t.noResults}</p>}
        {state.kind === 'done' && state.results.length > 0 && (
          <ul class="results" aria-label={t.resultsLabel}>
            {state.results.map((p) => (
              <li key={p.osm}>
                <a class="result" href={href.place(p.osm)}>
                  <IconPin class="result-icon" />
                  <span class="result-text">
                    <span class="result-name">{placeName(p, lang)}</span>
                    {altName(p, lang) && <span class="result-alt">{altName(p, lang)}</span>}
                    {adminLine(p, t.districtLine) && <span class="result-admin">{adminLine(p, t.districtLine)}</span>}
                  </span>
                  {p.covered === true && (
                    <span class="pill pill-covered" title={t.coveredHelp}>
                      {t.coveredBadge}
                    </span>
                  )}
                  <IconChevron class="chev" size={20} />
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
            <IconClock class="result-icon" />
            <span class="result-text">
              <span class="result-name">{placeName(last, lang)}</span>
            </span>
            <IconChevron class="chev" size={20} />
          </a>
        </section>
      )}

      <InstallButton />
    </div>
  );
}

/**
 * Name, promise and a quiet Himalayan scene: ridges, a glacial lake with the
 * ripple of an आहट, and the river it would flood. Pure inline SVG.
 */
function Hero() {
  const { lang } = useI18n();
  const other = lang === 'hi' ? 'en' : 'hi';
  return (
    <section class="hero" aria-labelledby="hero-h">
      <div class="hero-text">
        <h1 id="hero-h" class="hero-name" lang="hi">
          आहट
        </h1>
        <p class="hero-line">{STRINGS[lang].heroLine}</p>
        <p class="hero-line-alt" lang={other}>
          {STRINGS[other].heroLine}
        </p>
      </div>
      <svg class="hero-art" viewBox="0 22 360 128" preserveAspectRatio="xMidYMax slice" aria-hidden="true" focusable="false">
        <path
          fill="#bdcde0"
          d="M0 82 28 62l24 11 32-37 20 15 24-22 22 20 26-28 28 32 28-16 26 20 32-30 28 24 22-10 20 14v96H0Z"
        />
        <path
          fill="#fff"
          d="m76 45 8-9 9 10-5-2-4 4-4-3Zm44-7 8-8 9 9-5-1-4 3-3-3Zm46-8 10-12 11 13-6-2-5 5-5-4Zm114 6 10-10 9 11-5-2-4 4-4-4Z"
        />
        <path fill="#8ba3be" d="M0 110 36 88l30 12 38-24 34 16 20-6 18 8h24l18-12 32 12 36-22 34 18 40-10v76H0Z" />
        <ellipse cx="188" cy="99" rx="17" ry="4.2" fill="#43c2c0" />
        <g class="hero-ripple" fill="none" stroke="#43c2c0">
          <ellipse cx="188" cy="99" rx="27" ry="7" stroke-width="1.6" opacity=".7" />
          <ellipse cx="188" cy="99" rx="38" ry="10" stroke-width="1.3" opacity=".45" />
          <ellipse cx="188" cy="99" rx="50" ry="13" stroke-width="1" opacity=".25" />
        </g>
        <path fill="#56718f" d="M0 150v-30l30-10 34 8 36-14 40 14 32-6 12 38Zm212 0 6-34 30-8 34 14 38-16 40 12v32Z" />
        <path
          fill="none"
          stroke="#43c2c0"
          stroke-width="4.5"
          stroke-linecap="round"
          d="M188 103c-6 8 8 12 2 20s-12 14 4 27"
        />
        <path class="hero-ground" d="M0 150v-12c40-6 90 2 130-3 30-4 46 0 60 3 20 4 50-6 90-4 30 2 60 6 80 2v14Z" />
      </svg>
    </section>
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

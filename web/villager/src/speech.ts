// Speech-to-text for the Ask box, through the browser's own Web Speech API
// (Chrome and Android, Safari). Nothing is installed and no audio comes to our
// server: the browser's speech service turns it into text. Where the API is
// missing (Firefox) the mic button is simply not shown.

type Result = { isFinal: boolean; 0: { transcript: string } };
type ResultEvent = { resultIndex: number; results: ArrayLike<Result> };
type Recognition = {
  lang: string;
  interimResults: boolean;
  continuous: boolean;
  maxAlternatives: number;
  onresult: ((e: ResultEvent) => void) | null;
  onerror: ((e: { error: string }) => void) | null;
  onend: (() => void) | null;
  start(): void;
  stop(): void;
  abort(): void;
};

export type SpeechError = 'denied' | 'nothing' | 'failed';

export type Listener = { stop(): void; abort(): void };

function ctor(): (new () => Recognition) | null {
  const w = window as unknown as Record<string, unknown>;
  return (w.SpeechRecognition ?? w.webkitSpeechRecognition ?? null) as (new () => Recognition) | null;
}

export const speechSupported = (): boolean => typeof window !== 'undefined' && ctor() !== null;

/**
 * Listen for one spoken question. `onText` gets the transcript as it grows;
 * `onDone` gets the final text ('' if nothing usable was heard) or an error.
 */
export function listen(
  lang: 'hi' | 'en',
  onText: (text: string) => void,
  onDone: (text: string, error?: SpeechError) => void,
): Listener | null {
  const C = ctor();
  if (!C) return null;
  const rec = new C();
  rec.lang = lang === 'hi' ? 'hi-IN' : 'en-IN';
  rec.interimResults = true;
  rec.continuous = false;
  rec.maxAlternatives = 1;
  let text = '';
  let error: SpeechError | undefined;
  let finished = false;
  rec.onresult = (e) => {
    let all = '';
    for (let i = 0; i < e.results.length; i++) all += e.results[i][0].transcript;
    text = all.trim();
    onText(text);
  };
  rec.onerror = (e) => {
    if (e.error === 'not-allowed' || e.error === 'service-not-allowed') error = 'denied';
    else if (e.error === 'no-speech') error = 'nothing';
    else if (e.error !== 'aborted') error = 'failed';
  };
  rec.onend = () => {
    if (finished) return;
    finished = true;
    onDone(text, text ? undefined : error);
  };
  try {
    rec.start();
  } catch {
    return null;
  }
  return {
    stop: () => rec.stop(),
    abort: () => {
      finished = true;
      rec.abort();
    },
  };
}

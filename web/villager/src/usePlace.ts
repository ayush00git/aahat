import { useEffect, useState } from 'preact/hooks';
import { ApiError, getThreats, savedThreats, type Threats } from './api';
import { setLastPlace } from './storage';

export type PlaceState =
  | { kind: 'loading' }
  | { kind: 'error'; notFound: boolean; retry: () => void }
  | { kind: 'ok'; data: Threats; savedAt?: number };

// The answer for the last place viewed, so moving between the village, map
// and sign-up screens doesn't refetch on a slow connection.
let memo: { osm: string; data: Threats; savedAt?: number } | null = null;

export function usePlace(osm: string): PlaceState {
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<PlaceState>(() => {
    if (memo?.osm === osm) return { kind: 'ok', data: memo.data, savedAt: memo.savedAt };
    const saved = savedThreats(osm); // shown at once, refreshed below
    return saved ? { kind: 'ok', data: saved } : { kind: 'loading' };
  });

  useEffect(() => {
    if (memo?.osm === osm && attempt === 0 && !memo.savedAt) return;
    let live = true;
    getThreats(osm).then(
      ({ data, savedAt }) => {
        if (!live) return;
        memo = { osm, data, savedAt };
        if (data.known) setLastPlace({ osm, name: data.name, name_hi: data.name_hi });
        setState({ kind: 'ok', data, savedAt });
      },
      (err) => {
        if (!live) return;
        setState((s) =>
          s.kind === 'ok'
            ? s
            : {
                kind: 'error',
                notFound: err instanceof ApiError && err.status >= 400 && err.status < 500,
                retry: () => setAttempt((n) => n + 1),
              },
        );
      },
    );
    return () => {
      live = false;
    };
  }, [osm, attempt]);

  return state;
}

// localStorage that never throws (private mode, full disk, blocked storage).

const PREFIX = 'aahat:';

export function load<T>(key: string): T | null {
  try {
    const raw = localStorage.getItem(PREFIX + key);
    return raw ? (JSON.parse(raw) as T) : null;
  } catch {
    return null;
  }
}

export function save(key: string, value: unknown): void {
  try {
    localStorage.setItem(PREFIX + key, JSON.stringify(value));
  } catch {
    /* storage unavailable: the app still works, it just forgets */
  }
}

export function remove(key: string): void {
  try {
    localStorage.removeItem(PREFIX + key);
  } catch {
    /* ignore */
  }
}

// ---- Things the app remembers ---------------------------------------------

export interface RecentPlace {
  osm: string;
  name: string | null;
  name_hi: string | null;
}

export const getLastPlace = () => load<RecentPlace>('last-place');
export const setLastPlace = (p: RecentPlace) => save('last-place', p);

/** A subscription made from this phone, kept so it can be cancelled later. */
export interface SavedSubscription {
  id: string;
  place_osm: string;
  place_name: string;
  channel: string;
  phone: string;
  created_at: string;
}

export const getSubscriptions = () => load<SavedSubscription[]>('subscriptions') ?? [];

export function addSubscription(s: SavedSubscription): void {
  save('subscriptions', [...getSubscriptions().filter((x) => x.id !== s.id), s]);
}

export function removeSubscription(id: string): void {
  save('subscriptions', getSubscriptions().filter((x) => x.id !== id));
}

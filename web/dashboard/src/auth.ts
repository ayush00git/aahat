// Officials' token for the guarded routes (POST /trigger, GET /subscriptions,
// GET /events, GET /events/{id}). Kept in localStorage on this device; the
// server only checks it when it has a token configured.

import { useEffect, useState } from "preact/hooks";

const KEY = "aahat.officials.token";

let token: string | null = (() => {
  try {
    return localStorage.getItem(KEY);
  } catch {
    return null;
  }
})();
let prompt = false;
let promptReason: string | null = null;
let version = 0; // bumps on sign-in/out so views can refetch

type Listener = () => void;
const listeners = new Set<Listener>();
const emit = () => listeners.forEach((l) => l());

export function getToken(): string | null {
  return token;
}

export function signIn(t: string) {
  token = t.trim() || null;
  try {
    if (token) localStorage.setItem(KEY, token);
    else localStorage.removeItem(KEY);
  } catch {
    /* storage blocked: keep it for this tab only */
  }
  prompt = false;
  promptReason = null;
  version++;
  emit();
}

export function signOut() {
  token = null;
  try {
    localStorage.removeItem(KEY);
  } catch {
    /* ignore */
  }
  version++;
  emit();
}

/** Called by the API client when an officials' route answers 401. */
export function requireSignIn(reason: string) {
  prompt = true;
  promptReason = reason;
  emit();
}

export function dismissPrompt() {
  prompt = false;
  emit();
}

export interface AuthState {
  token: string | null;
  prompt: boolean;
  promptReason: string | null;
  version: number;
}

export function useAuth(): AuthState {
  const [, force] = useState(0);
  useEffect(() => {
    const l = () => force((n) => n + 1);
    listeners.add(l);
    return () => {
      listeners.delete(l);
    };
  }, []);
  return { token, prompt, promptReason, version };
}

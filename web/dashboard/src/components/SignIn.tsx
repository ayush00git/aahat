import { useEffect, useRef, useState } from "preact/hooks";
import { api } from "../api";
import { dismissPrompt, requireSignIn, signIn, signOut, useAuth } from "../auth";

/** Header widget: "Official signed in" with a sign-out link, or a discreet sign-in link. */
export function AuthStatus() {
  const auth = useAuth();
  if (auth.token) {
    return (
      <span class="auth-status">
        <span class="auth-dot" aria-hidden="true" /> Official signed in ·{" "}
        <button type="button" class="link-btn on-dark" onClick={signOut}>
          Sign out
        </button>
      </span>
    );
  }
  return (
    <span class="auth-status">
      <button type="button" class="link-btn on-dark" onClick={() => requireSignIn("")} title="For DDMA/SDMA staff">
        Sign in
      </button>
    </span>
  );
}

/** Modal opened from the header, or when an officials' route answers 401. */
export function SignInPrompt() {
  const auth = useAuth();
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const input = useRef<HTMLInputElement>(null);
  // Bumps whenever the dialog opens or closes, so a check that returns late is ignored.
  const attempt = useRef(0);

  useEffect(() => {
    attempt.current++;
    if (!auth.prompt) return;
    setValue("");
    setBusy(false);
    setError(null);
    setTimeout(() => input.current?.focus(), 0);
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && dismissPrompt();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [auth.prompt]);

  if (!auth.prompt) return null;
  const submit = async (e: Event) => {
    e.preventDefault();
    const t = value.trim();
    if (!t || busy) return;
    const mine = ++attempt.current;
    setBusy(true);
    setError(null);
    try {
      // The token is stored only once the server has accepted it.
      const ok = await api.checkToken(t);
      if (mine !== attempt.current) return;
      if (ok) {
        signIn(t);
        return;
      }
      setError("That token was not accepted. Check it and try again.");
    } catch (err) {
      if (mine !== attempt.current) return;
      setError(`Could not check the token: ${(err as Error).message}. Try again.`);
    }
    setBusy(false);
    input.current?.focus();
    input.current?.select();
  };
  return (
    <div class="modal-backdrop" onClick={dismissPrompt}>
      <form
        class="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="signin-title"
        aria-busy={busy}
        onClick={(e) => e.stopPropagation()}
        onSubmit={submit}
      >
        <h3 id="signin-title">Officials sign-in</h3>
        <p class="small">
          {auth.promptReason} Triggers, the alert log and subscriber lists are for DDMA/SDMA staff. Enter the officials'
          token you were given. It is kept in this browser until you sign out.
        </p>
        <label class="field">
          <span>Officials' token</span>
          <input
            ref={input}
            type="password"
            autoComplete="current-password"
            value={value}
            onInput={(e) => {
              setValue(e.currentTarget.value);
              setError(null);
            }}
            aria-invalid={error ? "true" : undefined}
            aria-describedby={error ? "signin-error" : undefined}
            readOnly={busy}
            required
          />
        </label>
        {error && (
          <p id="signin-error" class="small err-text" role="alert">
            {error}
          </p>
        )}
        <div class="modal-actions">
          <button type="button" class="btn" onClick={dismissPrompt}>
            Cancel
          </button>
          <button type="submit" class="btn-primary" disabled={!value.trim() || busy}>
            {busy ? "Checking…" : "Sign in"}
          </button>
        </div>
      </form>
    </div>
  );
}

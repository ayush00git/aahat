import { useEffect, useRef, useState } from "preact/hooks";
import { dismissPrompt, signIn, signOut, useAuth } from "../auth";

/** Header widget: sign-in link, or "Official" with a sign-out link. */
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
  return null;
}

/** Modal shown when an officials' route answers 401. */
export function SignInPrompt() {
  const auth = useAuth();
  const [value, setValue] = useState("");
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!auth.prompt) return;
    setValue("");
    setTimeout(() => input.current?.focus(), 0);
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && dismissPrompt();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [auth.prompt]);

  if (!auth.prompt) return null;
  const submit = (e: Event) => {
    e.preventDefault();
    if (value.trim()) signIn(value);
  };
  return (
    <div class="modal-backdrop" onClick={dismissPrompt}>
      <form
        class="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="signin-title"
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
            onInput={(e) => setValue(e.currentTarget.value)}
            required
          />
        </label>
        <div class="modal-actions">
          <button type="button" class="btn" onClick={dismissPrompt}>
            Cancel
          </button>
          <button type="submit" class="btn-primary" disabled={!value.trim()}>
            Sign in
          </button>
        </div>
      </form>
    </div>
  );
}

import { useEffect, useRef, useState } from "preact/hooks";
import { api, ApiError } from "../api";
import { requireSignIn, useAuth } from "../auth";
import { arrival, dateTime, discharge, KIND_LABEL, metres, num, OUTCOME_LABEL, signedMetres, unnamedLabel } from "../format";
import type { AlertEvent, DownstreamFile, Lake, ScenarioName } from "../types";
import { ErrorMsg, Loading, MoreToggle, Section, StatusBadge } from "./ui";

const SCENARIO_LABEL: Record<string, string> = { expected: "Expected", severe: "Severe" };

function AudioButton({ url }: { url: string }) {
  const audio = useRef<HTMLAudioElement | null>(null);
  const [playing, setPlaying] = useState(false);
  const [err, setErr] = useState(false);
  useEffect(() => () => audio.current?.pause(), []);
  const toggle = () => {
    if (!audio.current) {
      const a = new Audio(api.base + url);
      a.onended = () => setPlaying(false);
      a.onpause = () => setPlaying(false);
      a.onplay = () => setPlaying(true);
      a.onerror = () => {
        setErr(true);
        setPlaying(false);
      };
      audio.current = a;
    }
    const a = audio.current;
    if (a.paused) a.play().catch(() => setErr(true));
    else a.pause();
  };
  return (
    <button
      type="button"
      class={`icon-btn${err ? " err" : ""}`}
      onClick={toggle}
      title={err ? "Audio could not be played" : playing ? "Pause spoken message" : "Play spoken message"}
      aria-label={playing ? "Pause spoken message" : "Play spoken message"}
    >
      {playing ? (
        <svg viewBox="0 0 16 16" width="12" height="12" aria-hidden="true">
          <rect x="3" y="2" width="3.5" height="12" fill="currentColor" />
          <rect x="9.5" y="2" width="3.5" height="12" fill="currentColor" />
        </svg>
      ) : (
        <svg viewBox="0 0 16 16" width="12" height="12" aria-hidden="true">
          <path d="M4 2 L14 8 L4 14 Z" fill="currentColor" />
        </svg>
      )}
    </button>
  );
}

/** Events the server's refresh job raises itself when the satellite drain check flags a lake. */
const DRAIN_CHECK = "satellite_drain_check";
const DRAIN_CHECK_LABEL = "Raised automatically by the satellite drain check.";

function SourceTag({ source }: { source: string }) {
  if (source === "simulation") return <span class="tag sim">SIMULATION</span>;
  if (source === DRAIN_CHECK) return <span class="tag sim">SATELLITE CHECK</span>;
  return <span class="tag live">{source.toUpperCase()}</span>;
}

type Mode = "dry" | "live" | "unknown";

/**
 * Whether an event actually went out. Prefers the event's own dry_run flag,
 * then the delivery statuses (an older server ignores dry_run and sends).
 */
function eventMode(ev: AlertEvent): Mode {
  const rs = ev.recipients ?? [];
  if (rs.some((r) => r.delivery.status === "sent" || r.delivery.status === "failed")) return "live";
  if (ev.dry_run === true || (rs.length > 0 && rs.every((r) => r.delivery.status === "dry_run"))) return "dry";
  if (ev.dry_run === false) return "live";
  return "unknown";
}

function ModeTag({ mode }: { mode: Mode }) {
  if (mode === "dry")
    return (
      <span class="tag dry">
        <svg viewBox="0 0 16 16" width="11" height="11" aria-hidden="true">
          <path d="M6 1.5h4M6.8 1.5v4.2L2.6 13a1 1 0 0 0 .9 1.5h9a1 1 0 0 0 .9-1.5L9.2 5.7V1.5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" />
        </svg>
        DRY RUN
      </span>
    );
  if (mode === "live")
    return (
      <span class="tag live">
        <svg viewBox="0 0 16 16" width="11" height="11" aria-hidden="true">
          <circle cx="8" cy="8" r="2" fill="currentColor" />
          <path d="M4.5 4.5a5 5 0 0 0 0 7M11.5 4.5a5 5 0 0 1 0 7" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
        </svg>
        LIVE ALERT
      </span>
    );
  return null;
}

export function PlanView({ ev, requestedDry }: { ev: AlertEvent; requestedDry?: boolean }) {
  const places = ev.affected_places ?? [];
  const recips = ev.recipients ?? [];
  const subs = places.reduce((n, p) => n + p.subscribers, 0);
  const mode = eventMode(ev);
  const dryCount = recips.filter((r) => r.delivery.status === "dry_run").length;
  const [more, setMore] = useState(false);
  return (
    <div class="plan">
      {requestedDry && mode === "live" && (
        <p class="flag danger" role="alert">
          A dry run was requested but this server sent the messages: it does not support dry runs yet.
        </p>
      )}
      {mode === "dry" && (
        <p class="plan-banner dry">
          <b>Dry run.</b> Nothing was sent: {dryCount} planned message{dryCount === 1 ? "" : "s"} logged as "dry run".
        </p>
      )}
      {mode === "live" && (
        <p class="plan-banner live">
          <b>Live alert.</b> {ev.summary.sent} sent{ev.summary.failed ? `, ${ev.summary.failed} failed` : ""} to real phones.
        </p>
      )}
      <div class="plan-head">
        <ModeTag mode={mode} />
        <SourceTag source={ev.source} />
        <span>
          <b>{SCENARIO_LABEL[ev.scenario] ?? ev.scenario}</b> scenario · {dateTime(ev.created_at)}
        </span>
        <span class="muted small">{ev.event_id}</span>
      </div>
      {ev.source === DRAIN_CHECK && (
        <p class="small">{DRAIN_CHECK_LABEL} Nothing was sent: review it, then raise a live alert if it is real.</p>
      )}
      {ev.note && <p class="small">Note: {ev.note}</p>}
      {ev.sensor && (
        <p class="small">
          Sensor {ev.sensor.sensor_id}: {ev.sensor.kind}
          {ev.sensor.value !== null ? ` (${ev.sensor.value})` : ""}
        </p>
      )}
      <div class="tiles">
        <div class="tile">
          <div class="tile-v">{places.length}</div>
          <div class="tile-k">places to warn</div>
        </div>
        <div class="tile">
          <div class="tile-v">{subs}</div>
          <div class="tile-k">subscribers</div>
        </div>
        {mode === "dry" ? (
          <div class="tile">
            <div class="tile-v">{dryCount}</div>
            <div class="tile-k">planned, not sent</div>
          </div>
        ) : (
          <div class="tile">
            <div class="tile-v">{ev.summary.sent}</div>
            <div class="tile-k">sent</div>
          </div>
        )}
        <div class={`tile${ev.summary.failed ? " bad" : ""}`}>
          <div class="tile-v">{ev.summary.failed}</div>
          <div class="tile-k">failed</div>
        </div>
      </div>

      <div class="plan-sub">
        <h4>Affected places, in arrival order</h4>
        {places.length > 0 && <MoreToggle on={more} onToggle={() => setMore(!more)} />}
      </div>
      {places.length === 0 ? (
        <p class="muted small">No settlements, schools or health facilities are hit in this scenario.</p>
      ) : (
        <div class="table-scroll">
          <table class="table compact">
            <thead>
              <tr>
                <th class="num">#</th>
                <th class="num">km</th>
                <th>Place</th>
                <th class="num">Arrival</th>
                <th>Outcome</th>
                {more && <th class="num">Depth</th>}
                {more && <th class="num">Above flood</th>}
                <th class="num">Subs</th>
              </tr>
            </thead>
            <tbody>
              {places.map((p, i) => (
                <tr key={p.osm} class={p.subscribers === 0 ? "row-dim" : ""}>
                  <td class="num">{i + 1}</td>
                  <td class="num">{num(p.km, 1)}</td>
                  <td>
                    {p.name ?? <span class="muted">{unnamedLabel(p.kind)}</span>}
                    {p.name_hi && (
                      <span class="hi" lang="hi">
                        {" "}
                        {p.name_hi}
                      </span>
                    )}
                    {more && <span class="muted block small">{KIND_LABEL[p.kind] ?? p.kind}</span>}
                  </td>
                  <td class="num nowrap">{arrival(p.arrival_min_fast, p.arrival_min_expected)}</td>
                  <td>
                    <StatusBadge status={p.status} />
                    {more && p.scenario_status && (
                      <span class="muted block small">{OUTCOME_LABEL[p.scenario_status] ?? p.scenario_status}</span>
                    )}
                  </td>
                  {more && <td class="num nowrap">{metres(p.flood_depth_m)}</td>}
                  {more && <td class="num nowrap">{signedMetres(p.height_above_flood_m)}</td>}
                  <td class="num">{p.subscribers}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {places.some((p) => p.subscribers === 0) && (
        <p class="flag small">Places with no subscribers (dimmed) need warning by other means: phone tree, police, sirens.</p>
      )}

      <h4>Fan-out: messages per recipient</h4>
      {recips.length === 0 ? (
        <p class="muted small">No subscribers for the affected places.</p>
      ) : (
        <ul class="recipients">
          {recips.map((r) => (
            <li key={r.subscription_id + r.channel} class="recipient">
              <div class="recipient-head">
                <span class="recipient-place">{r.place_name || r.place_osm}</span>
                <span class="muted small">
                  {arrival(r.arrival_min_fast, r.arrival_min_expected)}
                </span>
                <span class={`delivery ${r.delivery.status}`} title={r.delivery.error ?? ""}>
                  {r.delivery.status === "dry_run" ? "dry run · not sent" : r.delivery.status}
                  {r.delivery.at ? ` ${dateTime(r.delivery.at).split(", ")[1] ?? ""}` : ""}
                </span>
              </div>
              <div class="recipient-meta small">
                <span>{r.phone || "—"}</span> · {r.channel} · {r.lang === "hi" ? "Hindi" : r.lang === "en" ? "English" : r.lang}
                {r.audio_url && <AudioButton url={r.audio_url} />}
              </div>
              {r.short_message && (
                <div class="msg sms" lang={r.lang}>
                  <span class="k">SMS</span> {r.short_message}
                </div>
              )}
              <details class="msg-full">
                <summary>Full message</summary>
                <div class="msg" lang={r.lang}>
                  {r.message}
                </div>
              </details>
              {r.delivery.error && <div class="small err-text">{r.delivery.error}</div>}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function EventRow({ ev }: { ev: AlertEvent }) {
  const [open, setOpen] = useState(false);
  const [full, setFull] = useState<AlertEvent | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const toggle = () => {
    const next = !open;
    setOpen(next);
    if (next && !full) {
      api
        .event(ev.event_id)
        .then(setFull)
        .catch((e) => setErr((e as Error).message));
    }
  };
  return (
    <li class={`event${open ? " open" : ""}`}>
      <button type="button" class="event-head" onClick={toggle} aria-expanded={open}>
        <ModeTag mode={eventMode(ev)} />
        <SourceTag source={ev.source} />
        <span class="event-when">{dateTime(ev.created_at)}</span>
        <span>{SCENARIO_LABEL[ev.scenario] ?? ev.scenario}</span>
        <span class="muted small">
          {ev.affected_places?.length ?? 0} places · {ev.summary.recipients} msgs · {ev.summary.sent} sent
          {ev.summary.failed ? ` · ${ev.summary.failed} failed` : ""}
          {eventMode(ev) === "dry" ? " · not sent" : ""}
        </span>
        {ev.source === DRAIN_CHECK && !open && (
          <span class="muted small event-why">
            {DRAIN_CHECK_LABEL} {ev.note}
          </span>
        )}
      </button>
      {open && (err ? <ErrorMsg msg={err} /> : full ? <PlanView ev={full} /> : <Loading what="event" />)}
    </li>
  );
}

export function AlertsTab({ lake, downstream }: { lake: Lake; downstream: DownstreamFile | null }) {
  const [scenario, setScenario] = useState<ScenarioName>("severe");
  const [note, setNote] = useState("");
  const [dryRun, setDryRun] = useState(true);
  // 0: closed; 1: first confirmation; 2: second confirmation (live alerts only).
  const [step, setStep] = useState<0 | 1 | 2>(0);
  const confirm = step > 0;
  const setConfirm = (open: boolean) => setStep(open ? 1 : 0);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ ev: AlertEvent; requestedDry: boolean } | null>(null);
  const auth = useAuth();
  const [err, setErr] = useState<string | null>(null);
  const [events, setEvents] = useState<AlertEvent[] | null>(null);
  const [eventsErr, setEventsErr] = useState<string | null>(null);
  const confirmBtn = useRef<HTMLButtonElement>(null);
  const cancelBtn = useRef<HTMLButtonElement>(null);

  const loadEvents = () => {
    setEventsErr(null);
    api
      .events(lake.id)
      .then((evs) => setEvents([...evs].sort((a, b) => b.created_at.localeCompare(a.created_at))))
      .catch((e) =>
        setEventsErr(e instanceof ApiError && e.status === 401 ? "signin" : (e as Error).message),
      );
  };

  useEffect(() => {
    setResult(null);
    setErr(null);
    setEvents(null);
    loadEvents();
  }, [lake.id]);

  // Signing in or out: refetch the log.
  useEffect(() => {
    if (auth.version > 0) loadEvents();
  }, [auth.version]);

  useEffect(() => {
    if (!confirm) return;
    // Live alerts: focus Cancel, so Enter doesn't send.
    if (dryRun) confirmBtn.current?.focus();
    else cancelBtn.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setConfirm(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [step]);

  const onConfirm = () => {
    if (!dryRun && step === 1) {
      setStep(2);
      return;
    }
    run();
  };

  const run = async () => {
    setStep(0);
    setBusy(true);
    setErr(null);
    const requestedDry = dryRun;
    try {
      const ev = await api.trigger(lake.id, scenario, note.trim(), requestedDry);
      setResult({ ev, requestedDry });
      loadEvents();
    } catch (e) {
      setErr(
        e instanceof ApiError && e.status === 401
          ? "Not run: sign in as an official, then try again."
          : `Trigger failed: ${(e as Error).message}`,
      );
    } finally {
      setBusy(false);
    }
  };

  const ds = lake.downstream;
  // The index's peak_m3s object is the older API's name for the same two numbers.
  const peak = (s: ScenarioName) =>
    downstream?.discharge.scenarios[s]?.peak_m3s ??
    (s === "expected" ? ds?.peak_expected_m3s : ds?.peak_severe_m3s) ??
    ds?.peak_m3s?.[s] ??
    null;
  // Small lakes: one relation gives both peaks, so either choice models the same flood.
  const sameFlood = downstream?.discharge.scenarios_identical ?? ds?.scenarios_identical ?? false;

  return (
    <div class="tab-body">
      <p class="sim-banner" role="note">
        <span class="tag sim">SIMULATION</span>
        Drill the warning chain for {lake.name}: who is warned, in which order.
      </p>

      <Section title="Simulate a burst">
        <div class="sim-form">
          <div class="seg seg-lg" role="radiogroup" aria-label="Scenario">
            {(["expected", "severe"] as ScenarioName[]).map((s) => (
              <button
                key={s}
                type="button"
                role="radio"
                aria-checked={scenario === s}
                class={scenario === s ? "on" : ""}
                onClick={() => setScenario(s)}
              >
                <span class="seg-title">{SCENARIO_LABEL[s]}</span>
                <span class="seg-sub">peak {discharge(peak(s))}</span>
              </button>
            ))}
          </div>
          {sameFlood && (
            <p class="caption">
              Expected and severe are the same flood for a lake this small: both give the same peak.
            </p>
          )}
          <label class="field">
            <span>Note for the log</span>
            <input
              type="text"
              maxLength={200}
              value={note}
              placeholder="e.g. DDMA Lahaul drill"
              onInput={(e) => setNote(e.currentTarget.value)}
            />
          </label>
          <label class={`check${dryRun ? "" : " live"}`}>
            <input type="checkbox" checked={dryRun} onChange={(e) => setDryRun(e.currentTarget.checked)} />
            <span class="check-text">
              <span>
                <b>Dry run</b> — don't send to real phones
              </span>
              <span class="check-sub">
                {dryRun
                  ? "The plan, messages and audio are built and logged; nothing is sent."
                  : "Unchecked: this run will message real subscribers."}
              </span>
            </span>
          </label>
          {!dryRun && (
            <p class="flag danger">
              <ModeTag mode="live" /> Messages will go to real subscribers' phones.
            </p>
          )}
          <button type="button" class={dryRun ? "btn-primary btn-lg" : "btn-danger"} disabled={busy} onClick={() => setConfirm(true)}>
            {busy ? "Running…" : dryRun ? "Simulate burst (dry run)" : "Simulate burst — LIVE"}
          </button>
        </div>
        {err && <ErrorMsg msg={err} />}
      </Section>

      {result && (
        <Section title="Fan-out plan (just now)">
          <PlanView ev={result.ev} requestedDry={result.requestedDry} />
        </Section>
      )}

      <Section
        title="Event log"
        aside={
          <button type="button" class="link-btn" onClick={loadEvents}>
            Refresh
          </button>
        }
      >
        {eventsErr === "signin" ? (
          <div class="state-msg">
            The alert log is for officials.{" "}
            <button type="button" class="link-btn" onClick={() => requireSignIn("")}>
              Sign in
            </button>
          </div>
        ) : eventsErr ? (
          <ErrorMsg msg={eventsErr} />
        ) : events === null ? (
          <Loading what="events" />
        ) : events.length === 0 ? (
          <p class="muted small">No events for this lake yet.</p>
        ) : (
          <ul class="events">
            {events.map((ev) => (
              <EventRow key={ev.event_id} ev={ev} />
            ))}
          </ul>
        )}
      </Section>

      {confirm && (
        <div class="modal-backdrop" onClick={() => setConfirm(false)}>
          <div
            class="modal"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="confirm-title"
            aria-describedby="confirm-desc"
            onClick={(e) => e.stopPropagation()}
          >
            <div class={`modal-mode ${dryRun ? "dry" : "live"}`}>
              <ModeTag mode={dryRun ? "dry" : "live"} />
              <span class="tag sim">SIMULATION</span>
              {!dryRun && <span class="small">Step {step} of 2</span>}
            </div>
            <h3 id="confirm-title">
              {step === 2
                ? `Send live warnings to real phones for ${lake.name}?`
                : `Simulate a ${SCENARIO_LABEL[scenario].toLowerCase()} burst of ${lake.name}?`}
            </h3>
            <div id="confirm-desc">
              {step === 1 && (
                <p>
                  The server builds the warning plan for the {SCENARIO_LABEL[scenario].toLowerCase()} scenario (peak{" "}
                  {discharge(peak(scenario))}) and logs it as a simulation.
                </p>
              )}
              {dryRun ? (
                <p class="flag">
                  Dry run: the plan, messages and audio are generated and logged, but <b>nothing is sent</b>. Each
                  recipient is recorded as "dry run".
                </p>
              ) : step === 1 ? (
                <p class="flag danger">
                  <b>Live alert.</b> Messages go to every subscriber of the affected places through the channels this server
                  has configured (SMS, web push, voice). You will be asked once more.
                </p>
              ) : (
                <p class="flag danger">
                  Subscribers' phones will receive a <b>real flood warning</b> for {lake.name}, even though this is a drill.
                  Only continue if subscribers have been told about the drill, or if this is a real emergency.
                </p>
              )}
            </div>
            <div class="modal-actions">
              <button type="button" class="btn" ref={cancelBtn} onClick={() => setConfirm(false)}>
                Cancel
              </button>
              <button type="button" class={dryRun ? "btn-primary" : "btn-danger"} ref={confirmBtn} onClick={onConfirm}>
                {dryRun ? "Run dry run" : step === 1 ? "Continue to send live…" : "Yes, send live alerts"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

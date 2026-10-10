import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { api } from "./api";
import { Footer, Header } from "./components/Chrome";
import { LakeList, rankLakes } from "./components/LakeList";
import { MapView } from "./components/MapView";
import { Panel, TAB_IDS, type LakeDetail, type TabId } from "./components/Panel";
import { SignInPrompt } from "./components/SignIn";
import { ErrorMsg, Loading } from "./components/ui";
import type { MapController } from "./map";
import { LAYER_NAMES, type Impact, type LakeIndex, type LakeLayers, type WeatherBrief } from "./types";

interface Loaded {
  detail: LakeDetail;
  layers: LakeLayers;
}

const cache = new Map<string, Promise<Loaded>>();

function errText(r: PromiseSettledResult<unknown>): string | undefined {
  return r.status === "rejected" ? String((r.reason as Error)?.message ?? r.reason) : undefined;
}

function loadLake(id: string): Promise<Loaded> {
  let p = cache.get(id);
  if (!p) {
    p = (async () => {
      // Weather (503 without an outlook) and the barrier scan (404 without one) are optional: shown when present.
      const [risk, downstream, impacts, weather, barrier, ...layerRes] = await Promise.allSettled([
        api.risk(id),
        api.downstream(id),
        api.impacts(id),
        api.lakeWeather(id),
        api.barrier(id),
        ...LAYER_NAMES.map((n) => api.layer(id, n)),
      ]);
      const layers: LakeLayers = {};
      const layerErrs: string[] = [];
      LAYER_NAMES.forEach((n, i) => {
        const r = layerRes[i];
        if (r.status === "fulfilled") layers[n] = r.value as LakeLayers[typeof n];
        else layerErrs.push(`${n}: ${errText(r)}`);
      });
      return {
        layers,
        detail: {
          risk: risk.status === "fulfilled" ? (risk.value as LakeDetail["risk"]) : null,
          downstream: downstream.status === "fulfilled" ? (downstream.value as LakeDetail["downstream"]) : null,
          impacts: impacts.status === "fulfilled" ? (impacts.value as Impact[]) : null,
          weather: weather.status === "fulfilled" ? (weather.value as LakeDetail["weather"]) : null,
          barrier: barrier.status === "fulfilled" ? (barrier.value as LakeDetail["barrier"]) : null,
          errors: {
            risk: errText(risk),
            downstream: errText(downstream),
            impacts: errText(impacts),
            layers: layerErrs.length ? layerErrs.join("; ") : undefined,
          },
        },
      };
    })();
    // Don't cache failures of the whole bundle (network down): allow retry.
    p.catch(() => cache.delete(id));
    cache.set(id, p);
  }
  return p;
}


/** Hash route: #/<lake-id>/<tab>. Works under any base path. */
function readHash(): { id: string | null; tab: TabId } {
  const [, id, tab] = location.hash.replace(/^#/, "").split("/");
  return {
    id: id ? decodeURIComponent(id) : null,
    tab: TAB_IDS.includes(tab as TabId) ? (tab as TabId) : "risk",
  };
}

export function App() {
  const [index, setIndex] = useState<LakeIndex | null>(null);
  const [indexErr, setIndexErr] = useState<string | null>(null);
  const initial = readHash();
  const [selectedId, setSelectedId] = useState<string | null>(initial.id);
  const [tab, setTab] = useState<TabId>(initial.tab);
  const [loaded, setLoaded] = useState<Loaded | null>(null);
  const [loading, setLoading] = useState(false);
  const ctrl = useRef<MapController | null>(null);
  const [weather, setWeather] = useState<Map<string, WeatherBrief>>(new Map());

  // Weather trigger levels for the lake list: optional, so a failure shows nothing.
  useEffect(() => {
    api
      .weather()
      .then((rows) => setWeather(new Map((Array.isArray(rows) ? rows : []).map((w) => [w.lake_id, w]))))
      .catch(() => {});
  }, []);

  const fetchIndex = () => {
    setIndexErr(null);
    api
      .lakes()
      .then(setIndex)
      .catch((e) => setIndexErr((e as Error).message));
  };
  useEffect(fetchIndex, []);

  // Keep the URL in step with the selection.
  useEffect(() => {
    const h = selectedId ? `#/${encodeURIComponent(selectedId)}/${tab}` : "";
    if (location.hash !== h) history.replaceState(null, "", h || location.pathname + location.search);
  }, [selectedId, tab]);
  useEffect(() => {
    const on = () => {
      const r = readHash();
      setSelectedId(r.id);
      setTab(r.tab);
    };
    window.addEventListener("hashchange", on);
    return () => window.removeEventListener("hashchange", on);
  }, []);

  const lakes = index?.lakes ?? [];
  const lake = lakes.find((l) => l.id === selectedId) ?? null;

  // Pick the top-ranked lake when nothing (or an unknown id) is selected.
  useEffect(() => {
    if (!index) return;
    if (!selectedId || !index.lakes.some((l) => l.id === selectedId)) {
      const top = rankLakes(index.lakes)[0];
      if (top) setSelectedId(top.id);
    }
  }, [index]);

  useEffect(() => {
    if (!lake) {
      setLoaded(null);
      return;
    }
    let live = true;
    setLoading(true);
    setLoaded(null);
    loadLake(lake.id)
      .then((l) => live && setLoaded(l))
      .catch((e) => {
        if (live)
          setLoaded({
            layers: {},
            detail: { risk: null, downstream: null, impacts: null, weather: null, barrier: null, errors: { risk: String(e), impacts: String(e) } },
          });
      })
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [lake?.id]);

  const select = useCallback((id: string) => setSelectedId(id), []);
  const onReady = useCallback((c: MapController) => {
    ctrl.current = c;
    // Dev-only handle for debugging and the smoke test.
    if (import.meta.env.DEV) (window as unknown as { __aahatMap: MapController }).__aahatMap = c;
  }, []);


  return (
    <div class={`app${lake ? " has-panel" : ""}`}>
      <Header lakes={index?.lakes ?? null} generatedAt={index?.generated_at ?? null} />
      {indexErr ? (
        <div class="sidebar">
          <ErrorMsg msg={`Could not load the lake index. ${indexErr}`} />
          <button type="button" class="btn" onClick={fetchIndex}>
            Retry
          </button>
        </div>
      ) : index ? (
        <LakeList lakes={lakes} selectedId={selectedId} onSelect={select} weather={weather} />
      ) : (
        <div class="sidebar">
          <Loading what="lakes" />
        </div>
      )}
      <main class="main">
        <MapView
          lakes={lakes}
          lake={lake}
          layers={loaded?.layers ?? null}
          impacts={loaded?.detail.impacts ?? null}
          loading={loading}
          onSelectLake={select}
          onReady={onReady}
        />
        {loaded?.detail.errors.layers && (
          <div class="map-error" role="alert">
            Some map layers failed: {loaded.detail.errors.layers}
          </div>
        )}
      </main>
      {lake && (
        <Panel
          lake={lake}
          detail={loaded?.detail ?? null}
          layers={loaded?.layers ?? null}
          loading={loading}
          tab={tab}
          onTab={setTab}
          onClose={() => {
            setSelectedId(null);
            ctrl.current?.fitAll(lakes);
          }}
          onFocusImpact={(im) => ctrl.current?.focusImpact(im)}
          onFocusPoint={(lon, lat) => ctrl.current?.focusPoint(lon, lat)}
        />
      )}
      <Footer apiBase={api.base} />
      <SignInPrompt />
    </div>
  );
}

import { render } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import './styles.css';
import { LangContext, STRINGS, type Lang } from './i18n';
import { load, save } from './storage';
import { useRoute, navigate } from './router';
import { registerServiceWorker } from './push';
import { Header } from './views/Header';
import { Footer } from './views/Footer';
import { Home } from './views/Home';
import { PlaceView } from './views/Place';
import { Subscribe } from './views/Subscribe';
import { MapScreen } from './views/MapScreen';
import { Alert, alertHash } from './views/Alert';

function initialLang(): Lang {
  const saved = load<Lang>('lang');
  return saved === 'en' || saved === 'hi' ? saved : 'hi';
}

function App() {
  const [lang, setLangState] = useState<Lang>(initialLang);
  const route = useRoute();
  const online = useOnline();

  const ctx = useMemo(
    () => ({
      lang,
      t: STRINGS[lang],
      setLang: (l: Lang) => {
        save('lang', l);
        setLangState(l);
      },
    }),
    [lang],
  );

  useEffect(() => {
    document.documentElement.lang = lang;
    document.title = lang === 'hi' ? 'आहट — बाढ़ चेतावनी' : 'Aahat — flood warning';
  }, [lang]);

  // A push arrived (or its notification was tapped) while the app is open.
  useEffect(() => {
    if (!('serviceWorker' in navigator)) return;
    const on = (e: MessageEvent) => {
      if (e.data?.type === 'aahat-alert') navigate(alertHash(e.data.payload));
    };
    navigator.serviceWorker.addEventListener('message', on);
    return () => navigator.serviceWorker.removeEventListener('message', on);
  }, []);

  if (route.name === 'alert') {
    return (
      <LangContext.Provider value={ctx}>
        <Alert params={route.params} />
      </LangContext.Provider>
    );
  }

  return (
    <LangContext.Provider value={ctx}>
      <Header home={route.name === 'home'} />
      {!online && (
        <p class="banner banner-offline" role="status">
          {ctx.t.offline}
        </p>
      )}
      <main id="main" class={route.name === 'home' ? 'main main-home' : 'main'}>
        {route.name === 'home' && <Home />}
        {route.name === 'place' && <PlaceView osm={route.osm} />}
        {route.name === 'subscribe' && <Subscribe osm={route.osm} />}
        {route.name === 'map' && <MapScreen osm={route.osm} />}
      </main>
      <Footer />
    </LangContext.Provider>
  );
}

function useOnline(): boolean {
  const [online, setOnline] = useState(navigator.onLine);
  useEffect(() => {
    const on = () => setOnline(navigator.onLine);
    window.addEventListener('online', on);
    window.addEventListener('offline', on);
    return () => {
      window.removeEventListener('online', on);
      window.removeEventListener('offline', on);
    };
  }, []);
  return online;
}

registerServiceWorker();
render(<App />, document.getElementById('app')!);

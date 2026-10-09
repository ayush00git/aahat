// UI text in Hindi and English. Safety advice lives in content/safety.ts.

import { createContext } from 'preact';
import { useContext } from 'preact/hooks';
import type { RiskLevel } from './api';

export type Lang = 'hi' | 'en';

const hi = {
  appName: 'आहट',
  tagline: 'हिमनद झील बाढ़ चेतावनी',
  langSwitch: 'भाषा',

  searchLabel: 'अपना गाँव खोजें',
  searchPlaceholder: 'गाँव का नाम, जैसे Sissu',
  searchHint: 'नाम अंग्रेज़ी अक्षरों में लिखने पर ज़्यादा गाँव मिलेंगे।',
  searching: 'खोज रहे हैं…',
  noResults:
    'कोई गाँव नहीं मिला। नाम अंग्रेज़ी अक्षरों में लिखकर देखें। अभी केवल उन झीलों के नीचे के गाँव शामिल हैं जिनकी हम निगरानी करते हैं।',
  searchError: 'खोज नहीं हो सकी। इंटरनेट जाँचें और फिर कोशिश करें।',
  resultsLabel: 'खोज के नतीजे',
  lastViewed: 'पिछली बार देखा गाँव',
  intro: 'देखें कि क्या आपका गाँव किसी हिमनद झील के फटने पर बाढ़ के रास्ते में है, और पानी पहुँचने में कितना समय लग सकता है।',

  back: 'वापस',
  loading: 'जानकारी ला रहे हैं…',
  retry: 'फिर कोशिश करें',
  loadError: 'जानकारी नहीं मिल सकी। इंटरनेट जाँचें और फिर कोशिश करें।',
  offline: 'इंटरनेट नहीं है।',
  savedCopy: (when: string) => `इंटरनेट नहीं है — ${when} की सहेजी जानकारी दिखा रहे हैं।`,
  install: 'फ़ोन पर ऐप जोड़ें',

  safeTitle: 'आपका गाँव किसी जोखिम वाले बाढ़ मार्ग में नहीं है',
  safeCaveat:
    'यह जानकारी केवल उन हिमनद झीलों के लिए है जिनकी हम निगरानी करते हैं, और गाँव के एक बिंदु पर आधारित है — नदी किनारे के घर, खेत और सड़कें फिर भी डूब सकते हैं। भारी बारिश, बादल फटने या दूसरी नदियों से बाढ़ फिर भी आ सकती है — ज़िला प्रशासन की चेतावनी हमेशा मानें।',
  unknownTitle: 'इस गाँव की जानकारी उपलब्ध नहीं',
  unknownBody: 'यह गाँव हमारी निगरानी वाली किसी झील के विश्लेषण में शामिल नहीं है।',
  dangerTitle: (n: number) => (n === 1 ? 'इस झील से बाढ़ का ख़तरा' : `इन ${n} झीलों से बाढ़ का ख़तरा`),

  badge_in_flood_path: 'बाढ़ मार्ग में',
  badge_at_risk: 'जोखिम में',
  lakeLine: (lake: string) => `${lake} से`,
  minutes: 'मिनट',
  arrivalLabel: 'पानी पहुँचने में लगभग समय (तेज़ अनुमान)',
  arrivalExpected: (m: number) => `सामान्य अनुमान: ~${m} मिनट`,
  arrivalUnknown: 'पानी पहुँचने के समय का अनुमान उपलब्ध नहीं',
  distance: (km: string) => `झील से नदी के रास्ते ${km} किमी`,
  floodTitle: 'अनुमानित बाढ़',
  scenario_expected: 'सामान्य स्थिति',
  scenario_severe: 'गंभीर स्थिति',
  riverDepth: (m: string) => `नदी में पानी तल से ${m} मीटर ऊँचा`,
  villageBelow: (m: string) => `गाँव की ज़मीन बाढ़ स्तर से ${m} मीटर नीचे`,
  villageAbove: (m: string) => `गाँव की ज़मीन बाढ़ स्तर से ${m} मीटर ऊपर`,
  villageAt: 'गाँव की ज़मीन लगभग बाढ़ स्तर पर',
  outcome_flooded: 'पानी गाँव तक आ सकता है',
  outcome_margin: 'पानी गाँव के किनारे तक आ सकता है',
  outcome_outside: 'गाँव तक पानी आने की संभावना कम',
  riskLabel: 'झील का जोखिम स्तर',
  risk: { low: 'कम', moderate: 'मध्यम', high: 'अधिक', very_high: 'बहुत अधिक' } as Record<RiskLevel, string>,

  openMap: 'नक्शे पर देखें',
  subscribeCta: 'चेतावनी पाने के लिए जुड़ें',
  subscribedHere: 'इस फ़ोन से इस गाँव के लिए चेतावनी चालू है',

  whatToDo: 'क्या करें',
  emergency: 'आपातकालीन नंबर',

  // Subscribe
  subscribeIntro: (place: string) => `${place} के लिए बाढ़ की चेतावनी आपके फ़ोन पर भेजी जाएगी।`,
  phoneLabel: 'मोबाइल नंबर',
  phoneHelp: '10 अंकों का नंबर लिखें',
  phoneInvalid: 'सही मोबाइल नंबर लिखें (+91 के बाद 10 अंक)',
  channelLabel: 'चेतावनी कैसे मिले?',
  channel_sms: 'SMS संदेश',
  channel_voice: 'फ़ोन कॉल',
  channel_webpush: 'ऐप सूचना',
  channelSoon: 'जल्द आ रहा है',
  channelPushHelp: 'इसी फ़ोन पर सूचना आएगी, नंबर ज़रूरी नहीं',
  messageLang: 'संदेश हिंदी में आएँगे।',
  consent: 'आपका नंबर केवल बाढ़ चेतावनी भेजने के लिए उपयोग होगा।',
  submit: 'जुड़ें',
  submitting: 'भेज रहे हैं…',
  success: 'आप जुड़ गए हैं',
  successBody: (place: string) => `बाढ़ का ख़तरा होने पर ${place} के लिए चेतावनी भेजी जाएगी।`,
  subId: 'पंजीकरण संख्या',
  unsubscribe: 'चेतावनी बंद करें',
  unsubscribing: 'बंद कर रहे हैं…',
  unsubscribed: 'चेतावनी बंद कर दी गई।',
  subError: 'जुड़ नहीं सके',
  pushDenied: 'सूचना की अनुमति नहीं मिली। फ़ोन की सेटिंग में इस ऐप के लिए सूचनाएँ चालू करें।',
  pushFailed: 'इस फ़ोन पर ऐप सूचना चालू नहीं हो सकी। SMS चुनें।',
  done: 'ठीक है',

  // Map
  mapTitle: 'नक्शा',
  mapData: 'नक्शा खोलने में ज़्यादा इंटरनेट डेटा लगता है।',
  mapLoading: 'नक्शा लोड हो रहा है…',
  mapError: 'नक्शा नहीं खुल सका।',
  mapLabel: (place: string) => `${place} और बाढ़ के रास्ते का नक्शा`,
  legend: 'नक्शे के चिह्न',
  legendVillage: 'आपका गाँव',
  legendLake: 'झील',
  legendPath: 'बाढ़ का रास्ता (नदी)',
  legendExpected: 'सामान्य बाढ़ क्षेत्र',
  legendSevere: 'गंभीर बाढ़ क्षेत्र',

  // Alert
  alertHeading: 'बाढ़ की चेतावनी',
  alertGo: 'तुरंत ऊँची जगह पर जाएँ — नदी से दूर',
  alertArrival: 'पानी पहुँचने में लगभग',
  alertPlay: 'संदेश सुनें',
  alertPause: 'रोकें',
  alertReceived: (time: string) => `चेतावनी मिली: ${time}`,
  alertClose: 'बंद करें',
  alertAudioError: 'आवाज़ वाला संदेश नहीं चल सका।',

  footerNote: 'स्क्रीनिंग अनुमान — उपग्रह आँकड़ों पर आधारित; आधिकारिक चेतावनी के लिए ज़िला प्रशासन का पालन करें।',
  sources: 'आँकड़ों के स्रोत',
};

export type Strings = typeof hi;

const en: Strings = {
  appName: 'आहट Aahat',
  tagline: 'Glacial lake flood warning',
  langSwitch: 'Language',

  searchLabel: 'Find your village',
  searchPlaceholder: 'Village name, e.g. Sissu',
  searchHint: 'Type the name in English letters.',
  searching: 'Searching…',
  noResults:
    'No village found. Try spelling the name in English letters. Only villages below the lakes we monitor are listed.',
  searchError: 'Search failed. Check your internet and try again.',
  resultsLabel: 'Search results',
  lastViewed: 'Last viewed village',
  intro: 'Check whether your village lies in the flood path if a glacial lake bursts, and how long the water could take to arrive.',

  back: 'Back',
  loading: 'Loading…',
  retry: 'Try again',
  loadError: 'Could not load. Check your internet and try again.',
  offline: 'No internet.',
  savedCopy: (when) => `No internet — showing information saved on ${when}.`,
  install: 'Add app to phone',

  safeTitle: 'Your village is not in any risky flood path',
  safeCaveat:
    'This covers only the glacial lakes we monitor and is judged at one point for the village — houses, fields and roads by the river can still flood. Floods from heavy rain, cloudbursts or other rivers can still happen — always follow district administration warnings.',
  unknownTitle: 'No information for this village',
  unknownBody: 'This village is not covered by the analysis of any lake we monitor.',
  dangerTitle: (n) => (n === 1 ? 'Flood danger from this lake' : `Flood danger from these ${n} lakes`),

  badge_in_flood_path: 'In flood path',
  badge_at_risk: 'At risk',
  lakeLine: (lake) => `From ${lake}`,
  minutes: 'min',
  arrivalLabel: 'Approximate time for the water to arrive (fast estimate)',
  arrivalExpected: (m) => `Typical estimate: ~${m} min`,
  arrivalUnknown: 'No estimate of arrival time',
  distance: (km) => `${km} km from the lake along the river`,
  floodTitle: 'Estimated flood',
  scenario_expected: 'Expected',
  scenario_severe: 'Severe',
  riverDepth: (m) => `River water ${m} m above the bed`,
  villageBelow: (m) => `Village ground ${m} m below flood level`,
  villageAbove: (m) => `Village ground ${m} m above flood level`,
  villageAt: 'Village ground about at flood level',
  outcome_flooded: 'Water may reach the village',
  outcome_margin: 'Water may reach the edge of the village',
  outcome_outside: 'Water unlikely to reach the village',
  riskLabel: 'Lake risk level',
  risk: { low: 'Low', moderate: 'Moderate', high: 'High', very_high: 'Very high' },

  openMap: 'See on map',
  subscribeCta: 'Sign up for warnings',
  subscribedHere: 'Warnings for this village are on for this phone',

  whatToDo: 'What to do',
  emergency: 'Emergency numbers',

  subscribeIntro: (place) => `Flood warnings for ${place} will be sent to your phone.`,
  phoneLabel: 'Mobile number',
  phoneHelp: 'Enter the 10-digit number',
  phoneInvalid: 'Enter a valid mobile number (10 digits after +91)',
  channelLabel: 'How should we warn you?',
  channel_sms: 'SMS',
  channel_voice: 'Phone call',
  channel_webpush: 'App notification',
  channelSoon: 'Coming soon',
  channelPushHelp: 'Notification on this phone, no number needed',
  messageLang: 'Messages will be in English.',
  consent: 'Your number is used only to send flood warnings.',
  submit: 'Sign up',
  submitting: 'Sending…',
  success: "You're signed up",
  successBody: (place) => `If there is a flood danger, a warning for ${place} will be sent.`,
  subId: 'Registration ID',
  unsubscribe: 'Stop warnings',
  unsubscribing: 'Stopping…',
  unsubscribed: 'Warnings stopped.',
  subError: 'Could not sign up',
  pushDenied: 'Notification permission was not given. Turn on notifications for this app in phone settings.',
  pushFailed: 'App notifications could not be turned on on this phone. Choose SMS instead.',
  done: 'OK',

  mapTitle: 'Map',
  mapData: 'Opening the map uses more internet data.',
  mapLoading: 'Loading map…',
  mapError: 'Could not open the map.',
  mapLabel: (place) => `Map of ${place} and the flood path`,
  legend: 'Map key',
  legendVillage: 'Your village',
  legendLake: 'Lake',
  legendPath: 'Flood path (river)',
  legendExpected: 'Expected flood area',
  legendSevere: 'Severe flood area',

  alertHeading: 'FLOOD WARNING',
  alertGo: 'Go to high ground now — away from the river',
  alertArrival: 'Water may arrive in about',
  alertPlay: 'Listen to message',
  alertPause: 'Pause',
  alertReceived: (time) => `Warning received: ${time}`,
  alertClose: 'Close',
  alertAudioError: 'The voice message could not be played.',

  footerNote:
    'Screening estimate based on satellite data; for official warnings follow the district administration.',
  sources: 'Data sources',
};

export const STRINGS: Record<Lang, Strings> = { hi, en };

export const LangContext = createContext<{ lang: Lang; t: Strings; setLang: (l: Lang) => void }>({
  lang: 'hi',
  t: hi,
  setLang: () => {},
});

export const useI18n = () => useContext(LangContext);

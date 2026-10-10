// UI text in Hindi and English. Safety advice lives in content/safety.ts.

import { createContext } from 'preact';
import { useContext } from 'preact/hooks';
import type { RiskLevel } from './api';

export type Lang = 'hi' | 'en';

/** "X झील", unless the Hindi name already says झील. */
const jheel = (lake: string) => (lake.includes('झील') ? lake : `${lake} झील`);
/** A sentence without its own final full stop / danda, so we can add ours. */
const bare = (s: string) => s.trim().replace(/[।.!]+$/, '');

const hi = {
  appName: 'आहट',
  tagline: 'हिमनद झील बाढ़ चेतावनी',
  heroLine: 'हिमनद झील बाढ़ की पहले से चेतावनी',
  langSwitch: 'भाषा',
  themeToDark: 'गहरा रंग चुनें',
  themeToLight: 'हल्का रंग चुनें',
  homeLabel: 'आहट — मुख्य पन्ना',

  searchLabel: 'अपना गाँव खोजें',
  searchPlaceholder: 'गाँव का नाम, जैसे Bhiyari',
  searchHint: 'नाम अंग्रेज़ी अक्षरों में लिखने पर ज़्यादा गाँव मिलेंगे।',
  searching: 'खोज रहे हैं…',
  noResults: 'कोई गाँव नहीं मिला। नाम अंग्रेज़ी अक्षरों में लिखकर देखें।',
  coveredBadge: 'निगरानी में',
  coveredHelp: 'निगरानी वाली झील के बाढ़ मार्ग का विश्लेषण इस गाँव तक पहुँचता है',
  searchError: 'खोज नहीं हो सकी। इंटरनेट जाँचें और फिर कोशिश करें।',
  districtLine: (d: string) => `${d} ज़िला`,
  weatherHigh: 'अगले 3 दिन भारी बारिश का अनुमान — सतर्क रहें',
  weatherElevated: 'अगले 3 दिन बारिश या गर्मी बढ़ने का अनुमान',
  weatherNormal: 'अगले 3 दिन मौसम सामान्य',
  weatherSource: 'स्रोत: Open-Meteo',
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
  nearbyVerdict: 'आपका गाँव बाढ़ के रास्ते से बाहर है, पर बाढ़ पास से गुज़रेगी — नदी से दूर रहें',
  notFoundTitle: 'यह जगह नहीं मिली',
  notFoundBody: 'यह लिंक पुराना या गलत हो सकता है। अपने गाँव का नाम खोजकर देखें।',
  notFoundSearch: 'गाँव खोजें',
  notCoveredTitle: 'यह गाँव हमारी निगरानी वाली किसी झील के बाढ़ मार्ग के नीचे नहीं है',
  notCoveredBody:
    'हम अभी चुनी हुई हिमनद झीलों पर उपग्रह से नज़र रखते हैं। निगरानी का दायरा बढ़ाया जा रहा है। भारी बारिश, बादल फटने या दूसरी नदियों से बाढ़ फिर भी आ सकती है — ज़िला प्रशासन की चेतावनी हमेशा मानें।',
  monitoredLakes: 'निगरानी वाली झीलें',
  nearestLakes: 'आपके पास की निगरानी वाली झीलें',
  nearestUncovered:
    'आपका गाँव इनमें से किसी झील के बाढ़ मार्ग पर नहीं है। ये झीलें पास हैं, पर हमारे विश्लेषण के अनुसार इनकी बाढ़ आपके गाँव तक नहीं पहुँचती।',
  nearestSafe: 'हमारे विश्लेषण के अनुसार इनमें से किसी झील की बाढ़ आपके गाँव के मुख्य स्थान तक नहीं पहुँचती।',
  straightKm: (km: string) => `~${km} किमी`,
  straightLabel: 'सीधी दूरी',
  straightNote: 'दूरी सीधी रेखा में (सीधी दूरी) है, नदी के रास्ते की नहीं।',
  lakesError: 'झीलों की सूची नहीं मिल सकी।',
  lakeRisk: (level: string) => `जोखिम: ${level}`,
  dangerTitle: (n: number) => (n === 1 ? 'इस झील से बाढ़ का खतरा' : `इन ${n} झीलों से बाढ़ का खतरा`),

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
  nearbyTitle: 'पास से गुज़रने वाली बाढ़',
  nearbyBadge: 'गाँव के पास से',
  nearbyText: (lake: string, min: number | null, h: number | null) =>
    `${lake} की बाढ़ आपके गाँव के पास से गुज़र सकती है` +
    (min !== null ? ` — नदी तक पानी ~${min} मिनट में` : '') +
    '।' +
    (h !== null ? ` गाँव का मुख्य स्थान अनुमानित बाढ़ स्तर से लगभग ${h} मीटर ऊपर है;` : '') +
    ' नदी किनारे के घर, खेत, सड़कें और पुल डूब सकते हैं।',
  nearbyRiver: (m: string) => `गाँव के मुख्य स्थान से नदी लगभग ${m} मीटर दूर`,
  risk: { low: 'कम', moderate: 'मध्यम', high: 'अधिक', very_high: 'बहुत अधिक' } as Record<RiskLevel, string>,

  openMap: 'नक्शे पर देखें',
  subscribeCta: 'चेतावनी पाने के लिए जुड़ें',
  subscribedHere: 'इस फ़ोन से इस गाँव के लिए चेतावनी चालू है',

  shareWhatsApp: 'WhatsApp पर भेजें',
  shareThreat: (lake: string, place: string, min: number | null) =>
    `आहट चेतावनी: ${jheel(lake)} से बाढ़ का खतरा।` +
    (min !== null ? ` ${place} तक पानी ~${min} मिनट में पहुँच सकता है।` : '') +
    ' ऊँचे स्थान पर जाएँ। जानकारी:',
  shareNearby: (lake: string, place: string, min: number | null) =>
    `आहट: ${jheel(lake)} की बाढ़ ${place} के पास से गुज़र सकती है` +
    (min !== null ? ` — नदी तक पानी ~${min} मिनट में` : '') +
    '। नदी किनारे से दूर रहें। जानकारी:',
  shareSafe: (place: string) =>
    `आहट: हमारे विश्लेषण के अनुसार ${place} किसी निगरानी वाली हिमनद झील के जोखिम वाले बाढ़ मार्ग में नहीं है। ज़िला प्रशासन की चेतावनी हमेशा मानें। जानकारी:`,
  shareAlert: (title: string, min: number | null) =>
    `आहट चेतावनी: ${bare(title)}।` +
    (min !== null ? ` पानी ~${min} मिनट में पहुँच सकता है।` : '') +
    ' ऊँचे स्थान पर जाएँ। जानकारी:',

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
  channelSmsHelp: 'किसी भी फ़ोन पर, इंटरनेट के बिना',
  channelVoiceHelp: 'फ़ोन कॉल पर बोलकर चेतावनी',
  stepOf: (n: number, total: number) => `चरण ${n} / ${total}`,
  confirmLabel: 'पुष्टि करें',
  messageLang: 'संदेश हिंदी में आएँगे।',
  consent: 'आपका नंबर केवल बाढ़ चेतावनी भेजने के लिए उपयोग होगा।',
  submit: 'जुड़ें',
  submitting: 'भेज रहे हैं…',
  success: 'आप जुड़ गए हैं',
  successBody: (place: string) => `बाढ़ का खतरा होने पर ${place} के लिए चेतावनी भेजी जाएगी।`,
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
  mapLakesLabel: 'निगरानी वाली झीलों का नक्शा',
  mapShow: 'नक्शा दिखाएँ',
  mapUnavailable: 'नक्शा अभी नहीं खुल सका। बाक़ी जानकारी इसके बिना भी मिलती रहेगी।',
  mapTwoFingers: 'नक्शा खिसकाने के लिए दो उँगलियाँ इस्तेमाल करें',
  mapScore: (score: string) => `जोखिम अंक: ${score} / 100`,
  basemapLabel: 'नक्शे का प्रकार',
  basemapSatellite: 'उपग्रह',
  basemapMap: 'नक्शा',
  officials: 'अधिकारी',

  // Ask Aahat
  askTitle: 'सवाल पूछें',
  askPlaceholder: 'अपना सवाल लिखें',
  askSend: 'सवाल भेजें',
  askExamplesLabel: 'सवालों के उदाहरण',
  askExamples: ['मेरे गाँव को किस झील से खतरा है?', 'बाढ़ आए तो क्या करूँ?', 'सबसे खतरनाक झील कौन सी है?'],
  askLoading: 'उत्तर तैयार हो रहा है… इसमें कुछ सेकंड लगते हैं।',
  askRateLimit: 'थोड़ी देर बाद पूछें।',
  askUnavailable: 'सहायक अभी उपलब्ध नहीं है।',
  askListen: 'उत्तर सुनें',
  askAudioLoading: 'आवाज़ लोड हो रही है…',
  askAudioError: 'आवाज़ नहीं चल सकी। फिर कोशिश करें।',
  askNote: 'उत्तर Aahat के आँकड़ों से; AI द्वारा लिखा गया',
  officialsLabel: 'अधिकारियों का डैशबोर्ड',

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
  heroLine: 'Early warning for glacial lake floods',
  langSwitch: 'Language',
  themeToDark: 'Switch to dark theme',
  themeToLight: 'Switch to light theme',
  homeLabel: 'Aahat home',

  searchLabel: 'Find your village',
  searchPlaceholder: 'Village name, e.g. Bhiyari',
  searchHint: 'Type the name in English letters.',
  searching: 'Searching…',
  noResults: 'No village found. Try spelling the name in English letters.',
  coveredBadge: 'Monitored',
  coveredHelp: 'The flood-path analysis of a monitored lake reaches this village',
  searchError: 'Search failed. Check your internet and try again.',
  districtLine: (d) => `${d} district`,
  weatherHigh: 'Heavy rain forecast in the next 3 days — stay alert',
  weatherElevated: 'Rain or a rise in heat forecast in the next 3 days',
  weatherNormal: 'Weather normal for the next 3 days',
  weatherSource: 'Source: Open-Meteo',
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
  nearbyVerdict: 'Your village is outside the mapped flood path, but a flood would pass close by — stay away from the river',
  notFoundTitle: 'Place not found',
  notFoundBody: 'This link may be old or wrong. Search for your village by name.',
  notFoundSearch: 'Search for a village',
  notCoveredTitle: 'This village is not below the flood path of any lake we monitor',
  notCoveredBody:
    'We currently watch selected glacial lakes by satellite, and coverage is expanding. Floods from heavy rain, cloudbursts or other rivers can still happen — always follow district administration warnings.',
  monitoredLakes: 'Lakes we monitor',
  nearestLakes: 'Monitored lakes near you',
  nearestUncovered:
    "Your village is not on the flood path of any of these lakes. They are nearby, but by our analysis their floods do not reach your village.",
  nearestSafe: "By our analysis, the flood from none of these lakes reaches your village's main point.",
  straightKm: (km) => `~${km} km`,
  straightLabel: 'straight-line',
  straightNote: 'Straight-line distance, not along the river.',
  lakesError: 'Could not load the list of lakes.',
  lakeRisk: (level) => `Risk: ${level}`,
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
  nearbyTitle: 'Floods passing close by',
  nearbyBadge: 'Near your village',
  nearbyText: (lake, min, h) =>
    `The flood from ${lake} may pass close to your village` +
    (min !== null ? ` — water could reach the river in ~${min} min` : '') +
    '.' +
    (h !== null ? ` The village's main point is about ${h} m above the estimated flood level;` : '') +
    ' houses, fields, roads and bridges by the river could flood.',
  nearbyRiver: (m) => `River about ${m} m from the village's main point`,
  risk: { low: 'Low', moderate: 'Moderate', high: 'High', very_high: 'Very high' },

  openMap: 'See on map',
  subscribeCta: 'Sign up for warnings',
  subscribedHere: 'Warnings for this village are on for this phone',

  shareWhatsApp: 'Send on WhatsApp',
  shareThreat: (lake, place, min) =>
    `Aahat warning: flood danger from ${lake}.` +
    (min !== null ? ` Water could reach ${place} in ~${min} min.` : '') +
    ' Move to high ground. Info:',
  shareNearby: (lake, place, min) =>
    `Aahat: the flood from ${lake} may pass close to ${place}` +
    (min !== null ? ` — water could reach the river in ~${min} min` : '') +
    '. Keep away from the riverbank. Info:',
  shareSafe: (place) =>
    `Aahat: by our analysis, ${place} is not in the risky flood path of any monitored glacial lake. Always follow district administration warnings. Info:`,
  shareAlert: (title, min) =>
    `Aahat warning: ${bare(title)}.` +
    (min !== null ? ` Water could arrive in ~${min} min.` : '') +
    ' Move to high ground. Info:',

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
  channelSmsHelp: 'Works on any phone, no internet needed',
  channelVoiceHelp: 'A spoken warning by phone call',
  stepOf: (n, total) => `Step ${n} of ${total}`,
  confirmLabel: 'Confirm',
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
  mapLakesLabel: 'Map of the lakes we monitor',
  mapShow: 'Show map',
  mapUnavailable: 'The map could not load right now. Everything else works without it.',
  mapTwoFingers: 'Use two fingers to move the map',
  mapScore: (score) => `Risk score: ${score} / 100`,
  basemapLabel: 'Map type',
  basemapSatellite: 'Satellite',
  basemapMap: 'Map',
  officials: 'Officials',

  askTitle: 'Ask a question',
  askPlaceholder: 'Type your question',
  askSend: 'Send question',
  askExamplesLabel: 'Example questions',
  askExamples: ['Which lake threatens my village?', 'What should I do if a flood comes?', 'Which lake is the most dangerous?'],
  askLoading: 'Preparing the answer… this takes a few seconds.',
  askRateLimit: 'Try again in a minute.',
  askUnavailable: 'The assistant is not available right now.',
  askListen: 'Listen to the answer',
  askAudioLoading: 'Loading audio…',
  askAudioError: 'The audio could not be played. Try again.',
  askNote: "Answer written by AI from Aahat's data",
  officialsLabel: "Officials' dashboard",

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

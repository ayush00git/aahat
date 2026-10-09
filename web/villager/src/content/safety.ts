// What to do in a glacial lake flood, and whom to call.
// Edit this file to change the advice; keep sentences short and plain.

import type { Lang } from '../i18n';

export const ACTIONS: Record<Lang, string[]> = {
  hi: [
    'चेतावनी मिलते ही तुरंत नदी से दूर, ऊँची जगह पर जाएँ। सामान समेटने में समय न गँवाएँ।',
    'पानी देखने नदी, नाले या पुल पर बिल्कुल न जाएँ। नदी किनारे की सड़क से भी दूर रहें।',
    'बुज़ुर्गों, बच्चों, गर्भवती महिलाओं, बीमार और दिव्यांग लोगों को साथ लेकर चलें।',
    'फ़ोन चार्ज रखें। टॉर्च, ज़रूरी दवाइयाँ और पहचान पत्र पास रखें।',
    'ज़िला प्रशासन (DDMA) और पुलिस के निर्देश मानें। अफ़वाहों पर ध्यान न दें।',
    'मदद के लिए 112 (राष्ट्रीय आपातकालीन नंबर) या 1077 (ज़िला आपदा नियंत्रण कक्ष) पर कॉल करें।',
  ],
  en: [
    'As soon as you are warned, move to high ground away from the river. Do not waste time packing.',
    'Never go to the river, stream or bridge to look at the water. Keep off riverside roads.',
    'Take the elderly, children, pregnant women, the sick and people with disabilities with you.',
    'Keep your phone charged. Keep a torch, essential medicines and ID with you.',
    'Follow the instructions of the district administration (DDMA) and police. Ignore rumours.',
    'For help call 112 (national emergency number) or 1077 (district disaster control room).',
  ],
};

export interface EmergencyNumber {
  number: string;
  label: Record<Lang, string>;
}

export const EMERGENCY_NUMBERS: EmergencyNumber[] = [
  { number: '112', label: { hi: 'आपातकालीन सहायता', en: 'Emergency' } },
  { number: '1077', label: { hi: 'ज़िला आपदा नियंत्रण कक्ष', en: 'District disaster control room' } },
];

export const CREDITS: Record<Lang, string[]> = {
  hi: [
    'उपग्रह चित्र: Sentinel-2, Copernicus (ESA / EU)',
    'गाँव, सड़कें और नक्शा: © OpenStreetMap योगदानकर्ता',
    'बाढ़ अनुमान का मिलान: NRSC (ISRO) के GLOF अध्ययन',
  ],
  en: [
    'Satellite imagery: Sentinel-2, Copernicus (ESA / EU)',
    'Villages, roads and map: © OpenStreetMap contributors',
    'Flood estimates benchmarked against NRSC (ISRO) GLOF studies',
  ],
};

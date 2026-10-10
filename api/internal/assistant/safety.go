package assistant

// What to do in a glacial lake flood, and whom to call. This mirrors
// web/villager/src/content/safety.ts: change both together.

const (
	LangHindi   = "hi"
	LangEnglish = "en"
)

var safetyActions = map[string][]string{
	LangHindi: {
		"चेतावनी मिलते ही तुरंत नदी से दूर, ऊँची जगह पर जाएँ। सामान समेटने में समय न गँवाएँ।",
		"पानी देखने नदी, नाले या पुल पर बिल्कुल न जाएँ। नदी किनारे की सड़क से भी दूर रहें।",
		"बुज़ुर्गों, बच्चों, गर्भवती महिलाओं, बीमार और दिव्यांग लोगों को साथ लेकर चलें।",
		"फ़ोन चार्ज रखें। टॉर्च, ज़रूरी दवाइयाँ और पहचान पत्र पास रखें।",
		"ज़िला प्रशासन (DDMA) और पुलिस के निर्देश मानें। अफ़वाहों पर ध्यान न दें।",
		"मदद के लिए 112 (राष्ट्रीय आपातकालीन नंबर) या 1077 (ज़िला आपदा नियंत्रण कक्ष) पर कॉल करें।",
	},
	LangEnglish: {
		"As soon as you are warned, move to high ground away from the river. Do not waste time packing.",
		"Never go to the river, stream or bridge to look at the water. Keep off riverside roads.",
		"Take the elderly, children, pregnant women, the sick and people with disabilities with you.",
		"Keep your phone charged. Keep a torch, essential medicines and ID with you.",
		"Follow the instructions of the district administration (DDMA) and police. Ignore rumours.",
		"For help call 112 (national emergency number) or 1077 (district disaster control room).",
	},
}

type emergencyNumber struct {
	Number string `json:"number"`
	Label  string `json:"label"`
}

var emergencyLabels = map[string][]emergencyNumber{
	LangHindi: {
		{"112", "आपातकालीन सहायता"},
		{"1077", "ज़िला आपदा नियंत्रण कक्ष"},
	},
	LangEnglish: {
		{"112", "Emergency"},
		{"1077", "District disaster control room"},
	},
}

// Short closing lines for the fallback answers.
var (
	highGround = map[string]string{
		LangHindi:   "चेतावनी मिलते ही तुरंत नदी से दूर, ऊँचे स्थान पर जाएँ।",
		LangEnglish: "If you are warned, move to high ground away from the river at once.",
	}
	callForHelp = map[string]string{
		LangHindi:   "मदद के लिए 112 या 1077 पर कॉल करें।",
		LangEnglish: "For help call 112 or 1077.",
	}
)

type safetyAdvice struct {
	Actions          []string          `json:"actions"`
	EmergencyNumbers []emergencyNumber `json:"emergency_numbers"`
}

func whatToDo(lang string) safetyAdvice {
	if _, ok := safetyActions[lang]; !ok {
		lang = LangHindi
	}
	return safetyAdvice{Actions: safetyActions[lang], EmergencyNumbers: emergencyLabels[lang]}
}

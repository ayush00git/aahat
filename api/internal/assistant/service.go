package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxQuestionChars = 500
	defaultTimeout   = 40 * time.Second
	defaultMaxTokens = 1000 // Devanagari costs several tokens a word: 500 cut Hindi answers mid-sentence
	defaultMaxRounds = 6
	audioTimeout     = 8 * time.Second
)

// ErrInvalid marks a request the caller must fix (HTTP 400).
var ErrInvalid = errors.New("invalid request")

// Speaker turns an answer into audio and returns its URL (voice.Polly; a fake in tests).
type Speaker interface {
	AudioURL(ctx context.Context, text, lang string) (string, error)
}

// Service answers questions. API is the Aahat HTTP API itself; the tools read through it.
type Service struct {
	Client  Converser
	Model   string
	API     http.Handler
	Speaker Speaker       // optional
	Timeout time.Duration // for one question, default 40 s
	Log     *slog.Logger
}

type Request struct {
	Question string `json:"question"`
	Lang     string `json:"lang"`
	PlaceOSM string `json:"place_osm"`
}

// Source is one lookup the answer rests on.
type Source struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

// Answer modes: written by the model and checked, or built from a fixed template.
const (
	ModeModel    = "model"
	ModeTemplate = "template"
)

type Answer struct {
	Answer   string   `json:"answer"`
	Lang     string   `json:"lang"`
	Sources  []Source `json:"sources"`
	AudioURL string   `json:"audio_url,omitempty"`
	// Mode says who wrote the answer: "model" (every number checked against the tool results) or
	// "template" (the model's answer failed that check twice, or the model could not be reached).
	Mode string `json:"mode"`
}

// systemPrompt takes the language rule, the risk-level words and the high-ground phrase of the
// answer's language, so an English conversation never sees a Hindi word.
const systemPrompt = `You are the question-answering voice of Aahat, a service that watches glacial lakes in Himachal Pradesh, India, for the danger of a glacial lake outburst flood (GLOF) and warns the villages downstream. The people asking are villagers, often with little schooling, reading on a basic phone or listening to your answer read aloud. A wrong number could cost lives or cause panic, so the rules about numbers below are absolute.

Language: %s Use short, everyday words a farmer would use. Write 2 to 5 short sentences of plain text: no lists, no headings, no markdown, no emoji.

Facts and numbers:
- You know nothing about any lake or village yourself. Everything you say about them must come from the data in this conversation: the tool results, and any data already looked up for you in the first message. Do not guess.
- Every number you write must appear in that data, copied exactly, decimals included (6.2 stays 6.2). Do not calculate, convert units, add, average, or round. The one exception: for minutes until water arrives, say the "minutes_to_say" value (already rounded down, the way our alerts say it).
- Use few numbers: the ones that answer the question.
- The emergency phone numbers 112 and 1077 may always be given.
- Risk levels: %s. A risk score is a screening score out of 100, not a chance of bursting.
- How deep the water would be: use if_lake_bursts from place_threats. river_water_depth_m is the depth of the flood water in the river beside the village, measured from the river bed; it is not the depth of water in the village. village_ground and village_ground_m say how far the village ground is below or above the flood level. Say both when asked about depth, and say which flood (expected, or the larger severe one) the figures are for.

Tools: each round of tool calls costs the person several seconds of waiting. If the first message already carries the data you need, answer straight away without calling any tool. Otherwise ask for every tool you need in the same turn, several calls at once, instead of one after another.

Finding the village: if the first message carries data for a selected village, that is the asker's village; use it and do not call place_threats for it again. If it only names a selected village (place_osm), call place_threats with it. If the person names another village, call search_places, then place_threats on the right match; if several different places match and you cannot tell which one, ask which district. If place_threats says known is false, say clearly that this village is not covered by our analysis and that we have no data for it; do not say it is safe. If a tool fails or a value is null or missing, say that this information is not available.

Never say or hint when a lake will burst, or whether it will: nobody can predict that. You can only say what the measurements show and how long water would take to arrive if a lake did burst.

Whenever the answer involves flood danger to a place or to people, end with the safety advice to move to high ground away from the river (%s), using the wording of the safety advice in the first message (the what_to_do tool returns the same text), and mention 112 or 1077 when help may be needed.

If the question is not about glacial lakes, floods, these villages, or staying safe, say in one sentence that you can only help with glacial lake flood questions.`

var languageRule = map[string]string{
	LangHindi: "Answer in simple Hindi in Devanagari script. Write digits as 0-9.",
	LangEnglish: "Answer in simple English only. Do not write a single Hindi word or Devanagari character, not even in " +
		"brackets after an English word; write village and lake names in Latin letters. This holds even if the " +
		"question itself is written in Hindi.",
}

var riskWords = map[string]string{
	LangHindi:   "low = कम, moderate = मध्यम, high = अधिक, very_high = बहुत अधिक",
	LangEnglish: "say low, moderate, high or very high",
}

var highGroundPhrase = map[string]string{
	LangHindi:   `"ऊँचे स्थान पर जाएँ"`,
	LangEnglish: `"move to high ground"`,
}

func systemFor(lang string) string {
	return fmt.Sprintf(systemPrompt, languageRule[lang], riskWords[lang], highGroundPhrase[lang])
}

// firstMessage is the opening user turn: the selected village and its threats when they could
// be looked up (so the model needs no tool round for the common question), the fixed safety
// advice, then the question.
func firstMessage(req Request, threats string) string {
	var b strings.Builder
	switch {
	case threats != "":
		fmt.Fprintf(&b, "[Selected village: place_osm = %s. Its data, already looked up (the result of place_threats for it):]\n%s\n\n", req.PlaceOSM, threats)
	case req.PlaceOSM != "":
		fmt.Fprintf(&b, "[Selected village: place_osm = %s]\n\n", req.PlaceOSM)
	}
	fmt.Fprintf(&b, "[Safety advice (the result of what_to_do):]\n%s\n\n[Question:]\n%s", encode(whatToDo(req.Lang)), req.Question)
	return b.String()
}

// Ask answers one question. It returns ErrInvalid for a bad request, and another error only
// when the model could not be reached and there is no data to build a template answer from.
func (s *Service) Ask(ctx context.Context, req Request) (*Answer, error) {
	req.Question = strings.TrimSpace(req.Question)
	req.PlaceOSM = strings.TrimSpace(req.PlaceOSM)
	if req.Lang == "" {
		req.Lang = LangHindi
	}
	switch {
	case req.Question == "":
		return nil, fmt.Errorf("%w: question is required", ErrInvalid)
	case utf8.RuneCountInString(req.Question) > MaxQuestionChars:
		return nil, fmt.Errorf("%w: question is longer than %d characters", ErrInvalid, MaxQuestionChars)
	case req.Lang != LangHindi && req.Lang != LangEnglish:
		return nil, fmt.Errorf("%w: lang must be \"hi\" or \"en\"", ErrInvalid)
	case req.PlaceOSM != "" && !osmID.MatchString(req.PlaceOSM):
		return nil, fmt.Errorf("%w: place_osm must look like node/123, way/123 or relation/123", ErrInvalid)
	}

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	modelCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tools := apiTools{api: s.API}
	exec := tools.exec(req.Lang)

	// The selected village's threats are fetched here rather than by the model: one model round
	// less for the most common question. The lookup counts as a call, so its numbers may be said
	// and it is listed in the sources.
	var prefetched []Call
	threats := ""
	if req.PlaceOSM != "" {
		args := map[string]any{"osm": req.PlaceOSM}
		if result, err := exec(modelCtx, toolPlaceThreats, args); err != nil {
			s.log().Warn("assistant could not look up the selected village", "osm", req.PlaceOSM, "err", err)
		} else {
			threats = result
			prefetched = append(prefetched, Call{Tool: toolPlaceThreats, Args: args, Result: result})
		}
	}
	conv := NewConversation(systemFor(req.Lang), askTools, firstMessage(req, threats))
	conv.Calls = prefetched
	runner := Runner{Client: s.Client, Model: s.Model, MaxTokens: defaultMaxTokens, MaxRounds: defaultMaxRounds}

	text, err := runner.Run(modelCtx, conv, exec)
	var bad problems
	if err == nil {
		if bad = check(conv, text, req.Lang); !bad.none() {
			s.log().Warn("assistant answer failed its checks, retrying", "numbers", bad.numbers, "devanagari", bad.devanagari, "empty", bad.empty)
			conv.Say(bad.correction())
			if text, err = runner.Run(modelCtx, conv, exec); err == nil {
				bad = check(conv, text, req.Lang)
			}
		}
	}

	ans := &Answer{Lang: req.Lang, Mode: ModeModel, Answer: text}
	if err != nil || !bad.none() {
		if err != nil {
			s.log().Error("assistant model call failed", "err", err)
			if len(conv.Calls) == 0 && req.PlaceOSM == "" {
				return nil, err
			}
		} else {
			s.log().Warn("assistant answer failed its checks twice, using the template", "numbers", bad.numbers, "devanagari", bad.devanagari, "empty", bad.empty)
		}
		// The template reads local data only, so it does not need what is left of the model's time.
		ans.Mode = ModeTemplate
		ans.Answer = s.fallback(ctx, req, conv, tools)
	}
	ans.Sources = sources(conv.Calls)

	if s.Speaker != nil {
		audioCtx, cancel := context.WithTimeout(ctx, audioTimeout)
		defer cancel()
		if u, err := s.Speaker.AudioURL(audioCtx, ans.Answer, req.Lang); err != nil {
			s.log().Warn("assistant answer not spoken", "err", err)
		} else {
			ans.AudioURL = u
		}
	}
	return ans, nil
}

func (s *Service) log() *slog.Logger {
	if s.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return s.Log
}

// problems are the reasons an answer may not be sent as written.
type problems struct {
	numbers    []string // numbers that are in no tool result
	devanagari bool     // Hindi text in an English answer
	empty      bool
}

func (p problems) none() bool { return len(p.numbers) == 0 && !p.devanagari && !p.empty }

// check tests an answer: every number must come from the conversation's tool results, and an
// English answer must not contain Devanagari.
func check(conv *Conversation, answer, lang string) problems {
	return problems{
		numbers:    NewAllowed(CallResults(conv.Calls)...).Unsupported(answer),
		devanagari: lang == LangEnglish && HasDevanagari(answer),
		empty:      answer == "",
	}
}

// HasDevanagari reports whether text contains any Devanagari character (letters, signs or digits).
func HasDevanagari(text string) bool {
	return strings.ContainsFunc(text, func(r rune) bool { return unicode.Is(unicode.Devanagari, r) })
}

// correction tells the model what was wrong with its answer, for the one retry.
func (p problems) correction() string {
	if p.empty {
		return "Your reply was empty. Answer the question now, following every rule."
	}
	var b strings.Builder
	b.WriteString("Your answer was not sent.")
	if len(p.numbers) > 0 {
		b.WriteString(" It contains numbers that are not in any tool result: " + strings.Join(p.numbers, ", ") +
			". Every number must be copied exactly from a tool result (call a tool if you need the figure); do not " +
			"calculate, convert or round. If the figure is not in the data, leave it out or say it is not available.")
	}
	if p.devanagari {
		b.WriteString(" It contains Hindi (Devanagari) text, but this person reads English only. Use English words " +
			"and Latin letters for everything, names included, with no Hindi in brackets.")
	}
	b.WriteString(" Write the answer again.")
	return b.String()
}

func sources(calls []Call) []Source {
	out := []Source{}
	for _, c := range calls {
		if !c.Failed {
			out = append(out, Source{Tool: c.Tool, Args: c.Args})
		}
	}
	return out
}

// lastResult decodes the latest successful result of a tool into v.
func lastResult(calls []Call, tool string, v any) bool {
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Tool == tool && !calls[i].Failed {
			return json.Unmarshal([]byte(calls[i].Result), v) == nil
		}
	}
	return false
}

var levelHindi = map[string]string{"low": "कम", "moderate": "मध्यम", "high": "अधिक", "very_high": "बहुत अधिक"}

// fallback builds an answer straight from the data, with no model text in it: about the village
// if one was looked up (or selected), else about the lake that was looked up, else only the
// safety advice.
func (s *Service) fallback(ctx context.Context, req Request, conv *Conversation, tools apiTools) string {
	hi := req.Lang == LangHindi
	closing := highGround[req.Lang] + " " + callForHelp[req.Lang]

	var pt placeThreats
	have := lastResult(conv.Calls, toolPlaceThreats, &pt)
	if !have && req.PlaceOSM != "" {
		args := map[string]any{"osm": req.PlaceOSM}
		if got, err := tools.placeThreats(ctx, req.PlaceOSM); err == nil {
			pt, have = *got, true
			conv.Calls = append(conv.Calls, Call{Tool: toolPlaceThreats, Args: args, Result: encode(got)})
		}
	}
	if have {
		return threatsText(pt, hi) + " " + closing
	}

	var lake struct {
		Name   string `json:"name"`
		NameHi string `json:"name_hi"`
		Risk   *struct {
			Score     *float64 `json:"score"`
			Level     string   `json:"level"`
			DataUntil string   `json:"data_until"`
		} `json:"risk"`
	}
	if lastResult(conv.Calls, toolLakeSummary, &lake) && lake.Risk != nil && lake.Risk.Level != "" {
		name, level := lake.Name, strings.ReplaceAll(lake.Risk.Level, "_", " ")
		if hi {
			if lake.NameHi != "" {
				name = lake.NameHi
			}
			if l, ok := levelHindi[lake.Risk.Level]; ok {
				level = l
			}
		}
		var b strings.Builder
		if hi {
			fmt.Fprintf(&b, "%s का जोखिम स्तर %s है", name, level)
			if lake.Risk.Score != nil {
				fmt.Fprintf(&b, " (स्कोर %s)", number(*lake.Risk.Score))
			}
			b.WriteString("। झील कब फटेगी, यह कोई नहीं बता सकता।")
		} else {
			fmt.Fprintf(&b, "The risk level of %s is %s", name, level)
			if lake.Risk.Score != nil {
				fmt.Fprintf(&b, " (score %s)", number(*lake.Risk.Score))
			}
			b.WriteString(". Nobody can say when a lake will burst.")
		}
		return b.String() + " " + closing
	}

	if hi {
		return "माफ़ कीजिए, इस सवाल का पक्का जवाब अभी मेरे पास नहीं है। अपने गाँव का नाम चुनकर दोबारा पूछें। " + closing
	}
	return "Sorry, I do not have a sure answer to this right now. Choose your village and ask again. " + closing
}

func threatsText(pt placeThreats, hi bool) string {
	place := deref(pt.Name)
	if hi && deref(pt.NameHi) != "" {
		place = deref(pt.NameHi)
	}
	if place == "" {
		place = map[bool]string{true: "यह जगह", false: "This place"}[hi]
	}
	lakeName := func(t placeThreat) string {
		if hi && t.LakeNameHi != "" {
			return t.LakeNameHi
		}
		return t.LakeName
	}
	switch {
	case !pt.Known:
		if hi {
			return place + " हमारे बाढ़ विश्लेषण में शामिल नहीं है, इसलिए इसके बारे में हमारे पास कोई आँकड़ा नहीं है।"
		}
		return place + " is not covered by our flood analysis, so we have no data for it."
	case len(pt.Threats) > 0:
		t := pt.Threats[0] // nearest in time
		if hi {
			when := "जल्द"
			if t.MinutesToSay != nil {
				when = "लगभग " + strconv.Itoa(*t.MinutesToSay) + " मिनट में"
			}
			return fmt.Sprintf("%s को %s से बाढ़ का खतरा है। झील फटने पर पानी %s पहुँच सकता है।", place, lakeName(t), when)
		}
		when := "soon"
		if t.MinutesToSay != nil {
			when = "in about " + strconv.Itoa(*t.MinutesToSay) + " minutes"
		}
		return fmt.Sprintf("%s is in flood danger from %s. If the lake bursts, water may arrive %s.", place, lakeName(t), when)
	case len(pt.Nearby) > 0:
		if hi {
			return fmt.Sprintf("%s अनुमानित बाढ़ मार्ग से बाहर है, पर %s का बाढ़ मार्ग पास से गुज़रता है। नदी किनारे सावधान रहें।", place, lakeName(pt.Nearby[0]))
		}
		return fmt.Sprintf("%s is outside the estimated flood path, but the flood path of %s passes nearby. Take care near the river.", place, lakeName(pt.Nearby[0]))
	default:
		if hi {
			return place + " किसी भी निगरानी वाली झील के अनुमानित बाढ़ मार्ग में नहीं है।"
		}
		return place + " is not in the estimated flood path of any monitored lake."
	}
}

func number(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

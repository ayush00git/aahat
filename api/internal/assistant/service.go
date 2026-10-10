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
	"unicode/utf8"
)

const (
	MaxQuestionChars = 500
	defaultTimeout   = 20 * time.Second
	defaultMaxTokens = 500
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
	Timeout time.Duration // for one question, default 20 s
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

const systemPrompt = `You are the question-answering voice of Aahat, a service that watches glacial lakes in Himachal Pradesh, India, for the danger of a glacial lake outburst flood (GLOF) and warns the villages downstream. The people asking are villagers, often with little schooling, reading on a basic phone or listening to your answer read aloud. A wrong number could cost lives or cause panic, so the rules about numbers below are absolute.

Language: %s Use short, everyday words a farmer would use. Write 2 to 5 short sentences of plain text: no lists, no headings, no markdown, no emoji.

Facts and numbers:
- You know nothing about any lake or village yourself. Everything you say about them must come from the tools in this conversation. Call the tools before answering; do not guess.
- Every number you write must appear in a tool result, copied exactly. Do not calculate, convert units, add, average, or round. The one exception: for minutes until water arrives, say the "minutes_to_say" value (already rounded down, the way our alerts say it).
- Use few numbers: the ones that answer the question.
- The emergency phone numbers 112 and 1077 may always be given.
- Risk levels: low = कम, moderate = मध्यम, high = अधिक, very_high = बहुत अधिक. A risk score is a screening score out of 100, not a chance of bursting.

Finding the village: if the message names a selected village (place_osm), call place_threats with it. If the person names a village, call search_places, then place_threats on the right match; if several different places match and you cannot tell which one, ask which district. If place_threats says known is false, say clearly that this village is not covered by our analysis and that we have no data for it; do not say it is safe. If a tool fails or a value is null, say that this information is not available.

Never say or hint when a lake will burst, or whether it will: nobody can predict that. You can only say what the measurements show and how long water would take to arrive if a lake did burst.

Whenever the answer involves flood danger to a place or to people, call what_to_do and end with its advice to move to high ground away from the river ("ऊँचे स्थान पर जाएँ"), using its wording, and mention 112 or 1077 when help may be needed.

If the question is not about glacial lakes, floods, these villages, or staying safe, say in one sentence that you can only help with glacial lake flood questions.`

var languageRule = map[string]string{
	LangHindi:   "Answer in simple Hindi in Devanagari script. Write digits as 0-9.",
	LangEnglish: "Answer in simple English.",
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

	user := req.Question
	if req.PlaceOSM != "" {
		user = "[Selected village: place_osm = " + req.PlaceOSM + "]\n" + user
	}
	conv := NewConversation(fmt.Sprintf(systemPrompt, languageRule[req.Lang]), askTools, user)
	runner := Runner{Client: s.Client, Model: s.Model, MaxTokens: defaultMaxTokens, MaxRounds: defaultMaxRounds}
	tools := apiTools{api: s.API}
	exec := tools.exec(req.Lang)

	text, err := runner.Run(modelCtx, conv, exec)
	var bad []string
	if err == nil {
		if bad = unsupported(conv, text); len(bad) > 0 || text == "" {
			s.log().Warn("assistant answer failed the number check, retrying", "numbers", bad)
			conv.Say(correction(bad))
			if text, err = runner.Run(modelCtx, conv, exec); err == nil {
				bad = unsupported(conv, text)
			}
		}
	}

	ans := &Answer{Lang: req.Lang, Mode: ModeModel, Answer: text}
	if err != nil || len(bad) > 0 || text == "" {
		if err != nil {
			s.log().Error("assistant model call failed", "err", err)
			if len(conv.Calls) == 0 && req.PlaceOSM == "" {
				return nil, err
			}
		} else {
			s.log().Warn("assistant answer failed the number check twice, using the template", "numbers", bad)
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

func unsupported(conv *Conversation, answer string) []string {
	return NewAllowed(CallResults(conv.Calls)...).Unsupported(answer)
}

func correction(bad []string) string {
	if len(bad) == 0 {
		return "Your reply was empty. Answer the question now, following every rule."
	}
	return "Your answer was not sent, because it contains numbers that are not in any tool result: " +
		strings.Join(bad, ", ") + ". Write the answer again. Every number must be copied exactly from a tool " +
		"result (call a tool if you need the figure); do not calculate, convert or round. If the figure is " +
		"not in the data, leave it out or say it is not available."
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

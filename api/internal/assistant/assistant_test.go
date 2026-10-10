package assistant_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/ayush00git/aahat/api/internal/alert"
	"github.com/ayush00git/aahat/api/internal/assistant"
	"github.com/ayush00git/aahat/api/internal/data"
	"github.com/ayush00git/aahat/api/internal/httpapi"
	"github.com/ayush00git/aahat/api/internal/store"
)

const (
	thirot   = "node/10928951156" // at risk from gepang-gath, arrival_min_fast 107.1
	bhiyari  = "node/8624123539"  // in the flood path of gepang-gath: 7.8 m / 17.8 m of water in the river, ground -1.2 m (severe)
	hamirpur = "node/1522750710"  // in the places index only: not covered by any lake
)

// fakeModel replays scripted Converse replies and records what it was sent.
type fakeModel struct {
	replies []types.Message
	err     error
	inputs  []*bedrockruntime.ConverseInput
}

func (f *fakeModel) Converse(_ context.Context, in *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	f.inputs = append(f.inputs, in)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.replies) == 0 {
		return nil, errors.New("fake model: no reply scripted")
	}
	msg := f.replies[0]
	f.replies = f.replies[1:]
	return &bedrockruntime.ConverseOutput{Output: &types.ConverseOutputMemberMessage{Value: msg}}, nil
}

func say(text string) types.Message {
	return types.Message{Role: types.ConversationRoleAssistant, Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: text}}}
}

func use(tool string, args map[string]any) types.Message {
	return types.Message{Role: types.ConversationRoleAssistant, Content: []types.ContentBlock{
		&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
			ToolUseId: aws.String("call-" + tool), Name: aws.String(tool), Input: document.NewLazyDocument(args),
		}},
	}}
}

// useAll is one model turn that asks for several tools at once.
func useAll(calls ...types.Message) types.Message {
	msg := types.Message{Role: types.ConversationRoleAssistant}
	for i, c := range calls {
		block := c.Content[0].(*types.ContentBlockMemberToolUse)
		block.Value.ToolUseId = aws.String(fmt.Sprintf("call-%d", i))
		msg.Content = append(msg.Content, block)
	}
	return msg
}

// firstUserText is the opening user message of the first request.
func (f *fakeModel) firstUserText() string {
	return f.inputs[0].Messages[0].Content[0].(*types.ContentBlockMemberText).Value
}

// toolResults returns the text of the tool results in the last message sent to the model.
func (f *fakeModel) toolResults(t *testing.T) []string {
	t.Helper()
	var out []string
	last := f.inputs[len(f.inputs)-1].Messages
	for _, block := range last[len(last)-1].Content {
		if tr, ok := block.(*types.ContentBlockMemberToolResult); ok {
			for _, c := range tr.Value.Content {
				if txt, ok := c.(*types.ToolResultContentBlockMemberText); ok {
					out = append(out, txt.Value)
				}
			}
		}
	}
	return out
}

func newService(t *testing.T, model *fakeModel) *assistant.Service {
	t.Helper()
	local, err := data.NewLocalStore("../httpapi/testdata")
	if err != nil {
		t.Fatal(err)
	}
	catalog := data.NewCatalog(local)
	st := store.NewMemoryStore()
	svc := alert.NewService(catalog, st, st, alert.NewDispatcher(alert.NewLogNotifier(nil)))
	api := httpapi.New(httpapi.Config{Catalog: catalog, Store: st, Alerts: svc})
	return &assistant.Service{Client: model, Model: "test-model", API: api}
}

func TestToolLoopAnswersFromData(t *testing.T) {
	model := &fakeModel{replies: []types.Message{
		use("search_places", map[string]any{"q": "Thirot"}),
		// Several tools asked for in one turn are all run before the model is called again.
		useAll(use("place_threats", map[string]any{"osm": thirot}), use("what_to_do", map[string]any{})),
		say("थिरोट को घेपन घाट झील से खतरा है। पानी लगभग 107 मिनट में पहुँच सकता है। तुरंत ऊँचे स्थान पर जाएँ। मदद के लिए 112 पर कॉल करें।"),
	}}
	s := newService(t, model)
	ans, err := s.Ask(context.Background(), assistant.Request{Question: "थिरोट गाँव को कितना खतरा है?"})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Mode != assistant.ModeModel || ans.Lang != "hi" || !strings.Contains(ans.Answer, "107 मिनट") {
		t.Fatalf("answer = %+v", ans)
	}
	if len(ans.Sources) != 3 || ans.Sources[0].Tool != "search_places" || ans.Sources[1].Tool != "place_threats" ||
		ans.Sources[1].Args["osm"] != thirot || ans.Sources[2].Tool != "what_to_do" {
		t.Fatalf("sources = %+v", ans.Sources)
	}
	if len(model.inputs) != 3 {
		t.Fatalf("model called %d times, want 3", len(model.inputs))
	}
	// The first request carries the question, the safety advice, the tools and the token cap.
	first := model.inputs[0]
	if got := model.firstUserText(); !strings.Contains(got, "थिरोट गाँव") || !strings.Contains(got, "ऊँची जगह पर जाएँ") || strings.Contains(got, "place_osm") {
		t.Errorf("user message %q", got)
	}
	if len(first.ToolConfig.Tools) != 5 || aws.ToInt32(first.InferenceConfig.MaxTokens) != 1000 {
		t.Errorf("tools = %d, max tokens = %d", len(first.ToolConfig.Tools), aws.ToInt32(first.InferenceConfig.MaxTokens))
	}
	if sys := first.System[0].(*types.SystemContentBlockMemberText).Value; !strings.Contains(sys, "in the same turn") || strings.Contains(sys, "%!") {
		t.Errorf("system prompt = %q", sys)
	}
	// The model saw the API's own numbers, plus the rounded-down minutes, and both results at once.
	res := model.toolResults(t)
	if len(res) != 2 || !strings.Contains(res[0], `"arrival_min_fast":107.1`) || !strings.Contains(res[0], `"minutes_to_say":107`) ||
		!strings.Contains(res[1], "1077") {
		t.Fatalf("tool results = %v", res)
	}
}

// With a village selected, its threats are looked up before the model is called: the model can
// answer in one call, and the depth of the flood is in what it reads.
func TestSelectedVillageIsLookedUpFirst(t *testing.T) {
	model := &fakeModel{replies: []types.Message{
		say("झील फटने पर नदी में पानी लगभग 17.8 मीटर गहरा होगा और गाँव की ज़मीन बाढ़ स्तर से 1.2 मीटर नीचे है। पानी लगभग 117 मिनट में पहुँच सकता है। तुरंत ऊँचे स्थान पर जाएँ।"),
	}}
	s := newService(t, model)
	ans, err := s.Ask(context.Background(), assistant.Request{Question: "पानी कितना गहरा होगा?", PlaceOSM: bhiyari})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Mode != assistant.ModeModel || !strings.Contains(ans.Answer, "17.8 मीटर") || len(model.inputs) != 1 {
		t.Fatalf("answer = %+v after %d model calls", ans, len(model.inputs))
	}
	if len(ans.Sources) != 1 || ans.Sources[0].Tool != "place_threats" || ans.Sources[0].Args["osm"] != bhiyari {
		t.Fatalf("sources = %+v", ans.Sources)
	}
	msg := model.firstUserText()
	for _, want := range []string{
		bhiyari, `"minutes_to_say":117`,
		// gepang-gath: flooded in both floods; the ground is 8.9 m above the expected flood level and 1.2 m below the severe one.
		`"expected":{"status":"flooded","river_water_depth_m":7.8,"village_ground":"above_flood_level","village_ground_m":8.9}`,
		`"severe":{"status":"flooded","river_water_depth_m":17.8,"village_ground":"below_flood_level","village_ground_m":1.2}`,
		// samudra-tapu, the second threat
		`"severe":{"status":"margin","river_water_depth_m":8.4,"village_ground":"above_flood_level","village_ground_m":3.5}`,
		"ऊँची जगह पर जाएँ", "पानी कितना गहरा होगा?",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("first message lacks %s:\n%s", want, msg)
		}
	}
	for _, unwanted := range []string{"flood_depth_m", "height_above_flood_m", "scenario_status", "-1.2"} {
		if strings.Contains(msg, unwanted) {
			t.Errorf("first message should not carry %s", unwanted)
		}
	}

	// The same figures reach the model when it calls the tool itself.
	model = &fakeModel{replies: []types.Message{
		use("place_threats", map[string]any{"osm": bhiyari}),
		say("The river water would be about 17.8 m deep and the village ground is 1.2 m below the flood level."),
	}}
	if ans, err = newService(t, model).Ask(context.Background(), assistant.Request{Question: "How deep in Bhiyari?", Lang: "en"}); err != nil || ans.Mode != assistant.ModeModel {
		t.Fatalf("answer = %+v, err %v", ans, err)
	}
	if res := model.toolResults(t); len(res) != 1 || !strings.Contains(res[0], `"river_water_depth_m":17.8`) || !strings.Contains(res[0], `"village_ground_m":1.2`) {
		t.Fatalf("place_threats result = %v", res)
	}
}

// A depth must be said as the data has it: a rounded-up or invented depth is caught like any number.
func TestDepthIsChecked(t *testing.T) {
	model := &fakeModel{replies: []types.Message{
		say("The river water would be about 18 m deep."), // 17.8 rounded up
		say("The river water would be about 17.8 m deep in the severe flood, and the village ground is 1.2 m below the flood level."),
	}}
	ans, err := newService(t, model).Ask(context.Background(), assistant.Request{Question: "How deep?", Lang: "en", PlaceOSM: bhiyari})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Mode != assistant.ModeModel || !strings.Contains(ans.Answer, "17.8 m") || len(model.inputs) != 2 {
		t.Fatalf("answer = %+v after %d model calls", ans, len(model.inputs))
	}
}

func TestEnglishConversationHasNoHindi(t *testing.T) {
	model := &fakeModel{replies: []types.Message{
		useAll(use("lake_summary", map[string]any{"lake_id": "gepang-gath"}), use("list_lakes", map[string]any{}),
			use("what_to_do", map[string]any{}), use("search_places", map[string]any{"q": "Hamirpur"})),
		say("Ghepan Ghat lake has a very high (बहुत अधिक) risk level. Move to high ground (ऊँचे स्थान पर जाएँ)."),
		say("Ghepan Ghat lake has a very high risk level. Move to high ground away from the river."),
	}}
	s := newService(t, model)
	ans, err := s.Ask(context.Background(), assistant.Request{Question: "How dangerous is my village?", Lang: "en", PlaceOSM: bhiyari})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Mode != assistant.ModeModel || assistant.HasDevanagari(ans.Answer) || len(model.inputs) != 3 {
		t.Fatalf("answer = %+v after %d model calls", ans, len(model.inputs))
	}
	// Nothing the model was sent has Hindi in it: prompt, first message, tool results, correction.
	last := model.inputs[2]
	if sys := last.System[0].(*types.SystemContentBlockMemberText).Value; assistant.HasDevanagari(sys) || !strings.Contains(sys, "English only") {
		t.Errorf("system prompt has Hindi or no English-only rule: %q", sys)
	}
	for _, m := range last.Messages {
		if m.Role != types.ConversationRoleUser {
			continue
		}
		for _, block := range m.Content {
			text := ""
			switch b := block.(type) {
			case *types.ContentBlockMemberText:
				text = b.Value
			case *types.ContentBlockMemberToolResult:
				text = b.Value.Content[0].(*types.ToolResultContentBlockMemberText).Value
			}
			if assistant.HasDevanagari(text) || strings.Contains(text, `_hi"`) {
				t.Errorf("Hindi sent to the model in an English conversation: %s", text)
			}
		}
	}
	fix := last.Messages[len(last.Messages)-1].Content[0].(*types.ContentBlockMemberText).Value
	if !strings.Contains(fix, "Devanagari") || strings.Contains(fix, "numbers that are not") {
		t.Errorf("correction = %q", fix)
	}
	// The tool results kept their numbers exactly.
	model.inputs = model.inputs[:2]
	if res := model.toolResults(t); len(res) != 4 || !strings.Contains(res[0], `"score":61.6`) || !strings.Contains(res[0], `"area_m2":1085800`) {
		t.Errorf("tool results = %v", res)
	}

	// A second answer with Hindi in it is replaced by the English template.
	model = &fakeModel{replies: []types.Message{say("खतरा है।"), say("Danger (खतरा).")}}
	ans, err = newService(t, model).Ask(context.Background(), assistant.Request{Question: "Is it safe?", Lang: "en", PlaceOSM: bhiyari})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Mode != assistant.ModeTemplate || assistant.HasDevanagari(ans.Answer) || !strings.Contains(ans.Answer, "Bhiyari is in flood danger") || !strings.Contains(ans.Answer, "117 minutes") {
		t.Fatalf("template answer = %+v", ans)
	}

	// Hindi answers are not affected by the check.
	model = &fakeModel{replies: []types.Message{say("खतरा है। ऊँचे स्थान पर जाएँ।")}}
	if ans, err = newService(t, model).Ask(context.Background(), assistant.Request{Question: "खतरा?", PlaceOSM: bhiyari}); err != nil || ans.Mode != assistant.ModeModel {
		t.Fatalf("hindi answer = %+v, err %v", ans, err)
	}
}

func TestInventedNumberIsCorrectedOnRetry(t *testing.T) {
	model := &fakeModel{replies: []types.Message{
		use("place_threats", map[string]any{"osm": thirot}),
		say("Water may reach Thirot in about 90 minutes. Move to high ground."), // 90 is in no tool result
		say("Water may reach Thirot in about 107 minutes. Move to high ground."),
	}}
	s := newService(t, model)
	ans, err := s.Ask(context.Background(), assistant.Request{Question: "How long do we have in Thirot?", Lang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Mode != assistant.ModeModel || !strings.Contains(ans.Answer, "107 minutes") {
		t.Fatalf("answer = %+v", ans)
	}
	msgs := model.inputs[2].Messages
	fix := msgs[len(msgs)-1].Content[0].(*types.ContentBlockMemberText).Value
	if !strings.Contains(fix, "90") {
		t.Fatalf("correction %q does not name the bad number", fix)
	}
}

func TestSecondBadAnswerFallsBackToTemplate(t *testing.T) {
	model := &fakeModel{replies: []types.Message{
		use("place_threats", map[string]any{"osm": thirot}),
		say("पानी 90 मिनट में पहुँचेगा।"),
		say("पानी 2 घंटे में पहुँचेगा।"),
	}}
	s := newService(t, model)
	ans, err := s.Ask(context.Background(), assistant.Request{Question: "कितना समय है?", PlaceOSM: thirot})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Mode != assistant.ModeTemplate {
		t.Fatalf("mode = %q, want template", ans.Mode)
	}
	for _, want := range []string{"Thirot", "घेपन घाट झील", "लगभग 107 मिनट में", "ऊँचे स्थान पर जाएँ", "112", "1077"} {
		if !strings.Contains(ans.Answer, want) {
			t.Errorf("template answer %q lacks %q", ans.Answer, want)
		}
	}
	if strings.Contains(ans.Answer, "90") {
		t.Errorf("template answer kept the invented number: %q", ans.Answer)
	}
}

func TestNotCoveredPlace(t *testing.T) {
	model := &fakeModel{replies: []types.Message{
		use("search_places", map[string]any{"q": "Hamirpur"}),
		use("place_threats", map[string]any{"osm": hamirpur}),
		say("हमीरपुर हमारे विश्लेषण में शामिल नहीं है, इसलिए इसके लिए हमारे पास कोई जानकारी नहीं है।"),
	}}
	s := newService(t, model)
	ans, err := s.Ask(context.Background(), assistant.Request{Question: "क्या हमीरपुर सुरक्षित है?"})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Mode != assistant.ModeModel || len(ans.Sources) != 2 {
		t.Fatalf("answer = %+v", ans)
	}
	// What the model was told about the place: found by search, but unknown to the analysis.
	model.inputs = model.inputs[:3]
	res := model.toolResults(t)
	var pt struct {
		Known bool   `json:"known"`
		Note  string `json:"note"`
	}
	if err := json.Unmarshal([]byte(res[0]), &pt); err != nil {
		t.Fatal(err)
	}
	if pt.Known || !strings.Contains(pt.Note, "not covered") {
		t.Fatalf("place_threats result = %s", res[0])
	}

	// With the model down, the template says the same thing instead of calling the place safe.
	down := &fakeModel{err: errors.New("bedrock unreachable")}
	ans, err = newService(t, down).Ask(context.Background(), assistant.Request{Question: "safe?", Lang: "en", PlaceOSM: hamirpur})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Mode != assistant.ModeTemplate || !strings.Contains(ans.Answer, "not covered") || strings.Contains(ans.Answer, "not in the estimated flood path") {
		t.Fatalf("template answer = %+v", ans)
	}
	if len(ans.Sources) != 1 || ans.Sources[0].Tool != "place_threats" {
		t.Fatalf("sources = %+v", ans.Sources)
	}
}

func TestModelDownWithoutDataIsAnError(t *testing.T) {
	s := newService(t, &fakeModel{err: errors.New("bedrock unreachable")})
	if _, err := s.Ask(context.Background(), assistant.Request{Question: "क्या खतरा है?"}); err == nil || errors.Is(err, assistant.ErrInvalid) {
		t.Fatalf("err = %v, want a model error", err)
	}
}

func TestToolRoundLimit(t *testing.T) {
	model := &fakeModel{}
	for range 10 {
		model.replies = append(model.replies, use("list_lakes", map[string]any{}))
	}
	s := newService(t, model)
	ans, err := s.Ask(context.Background(), assistant.Request{Question: "सब झीलें बताओ"})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.inputs) != 6 || ans.Mode != assistant.ModeTemplate {
		t.Fatalf("model called %d times (want 6), mode %q", len(model.inputs), ans.Mode)
	}
}

func TestLakeSummaryTool(t *testing.T) {
	model := &fakeModel{replies: []types.Message{
		use("lake_summary", map[string]any{"lake_id": "gepang-gath"}),
		use("lake_summary", map[string]any{"lake_id": "../etc"}),
		say("घेपन घाट झील का जोखिम स्तर बहुत अधिक है, स्कोर 61.6 है।"),
	}}
	s := newService(t, model)
	ans, err := s.Ask(context.Background(), assistant.Request{Question: "घेपन घाट झील कितनी खतरनाक है?"})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Mode != assistant.ModeModel || len(ans.Sources) != 1 { // the failed call is not a source
		t.Fatalf("answer = %+v", ans)
	}
	model.inputs = model.inputs[:2]
	res := model.toolResults(t)[0]
	for _, want := range []string{`"score":61.6`, `"level":"very_high"`, `"data_until":"2026-10-05"`, `"key":"volume"`, `"first_exposed"`, `"area_m2":1085800`} {
		if !strings.Contains(res, want) {
			t.Errorf("lake_summary result lacks %s:\n%s", want, res)
		}
	}
	for _, unwanted := range []string{`"why"`, `"sources"`, `"years"`} {
		if strings.Contains(res, unwanted) {
			t.Errorf("lake_summary result should not carry %s", unwanted)
		}
	}
}

func TestRequestValidation(t *testing.T) {
	s := newService(t, &fakeModel{})
	for name, req := range map[string]assistant.Request{
		"empty":    {Question: "  "},
		"too long": {Question: strings.Repeat("क", 501)},
		"bad lang": {Question: "hi", Lang: "fr"},
		"bad osm":  {Question: "hi", PlaceOSM: "village/1"},
	} {
		if _, err := s.Ask(context.Background(), req); !errors.Is(err, assistant.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestNumberCheck(t *testing.T) {
	allowed := assistant.NewAllowed(
		`{"arrival_min_fast":19.6,"km":11.53,"area_m2":1085800.0,"data_until":"2026-10-05","bounds":[605,123]}`,
		`{"note":"10 seasons 2017-2026: +26,062 m²/yr"}`,
	)
	pass := []string{
		"पानी 19 मिनट में पहुँचेगा", // rounded down
		"19.6 minutes, 11.53 km",         // exact
		"१९ मिनट",                        // Devanagari digits
		"area 1,085,800 m2 or 10,85,800", // grouping
		"5 अक्टूबर 2026 तक का डेटा",      // date parts, leading zero dropped
		"112 या 1077 पर कॉल करें",        // emergency numbers
		"26,062 m² हर साल, 2017 से 2026", // grouped number from a note
		"605 और 123",                     // JSON list read as two numbers
		"कोई संख्या नहीं",
	}
	for _, s := range pass {
		if bad := allowed.Unsupported(s); len(bad) != 0 {
			t.Errorf("%q: unexpected unsupported numbers %v", s, bad)
		}
	}
	// Depths and heights are decimals: said exactly (in either script, with a unit, at the end of a
	// sentence, with the sign the data had), or as the whole number below.
	depths := assistant.NewAllowed(`{"river_water_depth_m":6.2,"village_ground_m":1.2}`, `{"flood_depth_m":15.4,"height_above_flood_m":-1.2,"km":70.08}`)
	for _, s := range []string{
		"river water about 6.2 m deep; the village ground is 1.2 m below the flood level",
		"नदी में पानी 15.4 मीटर गहरा होगा।", "पानी ६.२ मीटर", "The depth is 6.2.", "6.20 m", "about 6 m", "15 m", "1 m", "70.08 km", "-1.2 m", "1.2m",
	} {
		if bad := depths.Unsupported(s); len(bad) != 0 {
			t.Errorf("%q: unexpected unsupported numbers %v", s, bad)
		}
	}
	for s, want := range map[string]string{
		"about 7 m deep": "7", "16 m": "16", "2 m below": "2", // rounded up
		"6.3 m": "6.3", "15.45 m": "15.45", "1.5 m": "1.5", "70.1 km": "70.1", "6,2 m": "2", // a decimal comma is not read as 6.2
	} {
		if bad := depths.Unsupported(s); len(bad) != 1 || bad[0] != want {
			t.Errorf("%q: unsupported = %v, want [%s]", s, bad, want)
		}
	}
	fail := map[string]string{
		"पानी 20 मिनट में":       "20", // rounded up
		"लगभग 1.08 वर्ग किमी":    "1.08",
		"2030 तक झील फट सकती है": "2030",
		"12 km": "12",
	}
	for s, want := range fail {
		if bad := allowed.Unsupported(s); len(bad) != 1 || bad[0] != want {
			t.Errorf("%q: unsupported = %v, want [%s]", s, bad, want)
		}
	}
}

type fakeSpeaker struct{ text, lang string }

func (f *fakeSpeaker) AudioURL(_ context.Context, text, lang string) (string, error) {
	f.text, f.lang = text, lang
	return "/audio/0123456789abcdef.mp3", nil
}

func post(h http.Handler, body any, remote, xff string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(b))
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandlerAnswerWithAudio(t *testing.T) {
	model := &fakeModel{replies: []types.Message{say("मैं केवल हिमनद झील की बाढ़ से जुड़े सवालों में मदद कर सकता हूँ।")}}
	s := newService(t, model)
	speaker := &fakeSpeaker{}
	s.Speaker = speaker
	rec := post(assistant.Handler(s, 0), map[string]string{"question": "क्रिकेट का स्कोर?"}, "203.0.113.9:4000", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var ans assistant.Answer
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if ans.AudioURL != "/audio/0123456789abcdef.mp3" || speaker.text != ans.Answer || speaker.lang != "hi" || ans.Sources == nil {
		t.Fatalf("answer = %+v, spoken %q (%s)", ans, speaker.text, speaker.lang)
	}
	if rec := post(assistant.Handler(s, 0), map[string]string{"question": ""}, "203.0.113.9:4000", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty question: status = %d", rec.Code)
	}
	if rec := post(assistant.NotConfigured(), map[string]string{"question": "x"}, "203.0.113.9:4000", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("not configured: status = %d", rec.Code)
	}
}

func TestRateLimitPerClient(t *testing.T) {
	model := &fakeModel{}
	for range 10 {
		model.replies = append(model.replies, say("ठीक है।"))
	}
	h := assistant.Handler(newService(t, model), 3)
	q := map[string]string{"question": "नमस्ते"}
	// Behind the local proxy the client is the last X-Forwarded-For entry; a spoofed first entry does not help.
	for i := range 3 {
		if rec := post(h, q, "127.0.0.1:5000", "10.9.9.9, 198.51.100.7"); rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d: %s", i, rec.Code, rec.Body)
		}
	}
	rec := post(h, q, "127.0.0.1:5000", "10.1.1.1, 198.51.100.7")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("4th request: status = %d, Retry-After = %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := post(h, q, "127.0.0.1:5000", "198.51.100.8"); rec.Code != http.StatusOK {
		t.Fatalf("another client: status = %d", rec.Code)
	}
	if len(model.inputs) != 4 {
		t.Fatalf("model called %d times, want 4 (the limited request must not reach it)", len(model.inputs))
	}
}

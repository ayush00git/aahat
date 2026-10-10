package assistant_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
		use("place_threats", map[string]any{"osm": thirot}),
		use("what_to_do", map[string]any{}),
		say("थिरोट को घेपन घाट झील से खतरा है। पानी लगभग 107 मिनट में पहुँच सकता है। तुरंत ऊँचे स्थान पर जाएँ। मदद के लिए 112 पर कॉल करें।"),
	}}
	s := newService(t, model)
	ans, err := s.Ask(context.Background(), assistant.Request{Question: "मेरे गाँव को कितना खतरा है?", PlaceOSM: thirot})
	if err != nil {
		t.Fatal(err)
	}
	if ans.Mode != assistant.ModeModel || ans.Lang != "hi" || !strings.Contains(ans.Answer, "107 मिनट") {
		t.Fatalf("answer = %+v", ans)
	}
	if len(ans.Sources) != 2 || ans.Sources[0].Tool != "place_threats" || ans.Sources[0].Args["osm"] != thirot || ans.Sources[1].Tool != "what_to_do" {
		t.Fatalf("sources = %+v", ans.Sources)
	}
	if len(model.inputs) != 3 {
		t.Fatalf("model called %d times, want 3", len(model.inputs))
	}
	// The first request carries the selected village, the tools and the token cap.
	first := model.inputs[0]
	if got := first.Messages[0].Content[0].(*types.ContentBlockMemberText).Value; !strings.Contains(got, thirot) {
		t.Errorf("user message %q does not name the selected village", got)
	}
	if len(first.ToolConfig.Tools) != 5 || aws.ToInt32(first.InferenceConfig.MaxTokens) != 500 {
		t.Errorf("tools = %d, max tokens = %d", len(first.ToolConfig.Tools), aws.ToInt32(first.InferenceConfig.MaxTokens))
	}
	// The model saw the API's own numbers, plus the rounded-down minutes.
	model.inputs = model.inputs[:2]
	res := model.toolResults(t)
	if len(res) != 1 || !strings.Contains(res[0], `"arrival_min_fast":107.1`) || !strings.Contains(res[0], `"minutes_to_say":107`) {
		t.Fatalf("place_threats result = %v", res)
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

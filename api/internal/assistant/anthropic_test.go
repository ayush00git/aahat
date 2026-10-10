package assistant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A tool-use conversation through the Anthropic adapter: the request carries the key, system
// prompt, tools and the tool result; the tool call and final text come back through the Runner.
func TestAnthropicToolLoop(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "k" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing auth headers: %v", r.Header)
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
		if len(bodies) == 1 {
			io.WriteString(w, `{"content":[{"type":"tool_use","id":"tu_1","name":"lake_summary","input":{"lake_id":"gepang-gath"}}],"usage":{"input_tokens":10,"output_tokens":5}}`)
			return
		}
		io.WriteString(w, `{"content":[{"type":"text","text":"score 61"}],"usage":{"input_tokens":20,"output_tokens":5}}`)
	}))
	defer srv.Close()

	tools := []Tool{{Name: "lake_summary", Description: "d", Schema: map[string]any{
		"type": "object", "properties": map[string]any{"lake_id": map[string]any{"type": "string"}},
	}}}
	c := NewConversation("be brief", tools, "how risky?")
	r := Runner{Client: &Anthropic{Key: "k", URL: srv.URL}, Model: "claude-test", MaxTokens: 100, MaxRounds: 3}
	text, err := r.Run(context.Background(), c, func(_ context.Context, name string, args map[string]any) (string, error) {
		if name != "lake_summary" || args["lake_id"] != "gepang-gath" {
			t.Errorf("tool call = %s %v", name, args)
		}
		return `{"score":61}`, nil
	})
	if err != nil || text != "score 61" {
		t.Fatalf("Run = %q, %v", text, err)
	}
	if len(bodies) != 2 {
		t.Fatalf("requests = %d", len(bodies))
	}
	first := bodies[0]
	if first["model"] != "claude-test" || first["system"] != "be brief" || first["max_tokens"] != float64(100) {
		t.Errorf("first request = %v", first)
	}
	tool := first["tools"].([]any)[0].(map[string]any)
	if tool["name"] != "lake_summary" || tool["input_schema"].(map[string]any)["type"] != "object" {
		t.Errorf("tool = %v", tool)
	}
	msgs := bodies[1]["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("second request messages = %d", len(msgs))
	}
	use := msgs[1].(map[string]any)["content"].([]any)[0].(map[string]any)
	if use["type"] != "tool_use" || use["id"] != "tu_1" || use["input"].(map[string]any)["lake_id"] != "gepang-gath" {
		t.Errorf("tool_use echoed as %v", use)
	}
	res := msgs[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if res["type"] != "tool_result" || res["tool_use_id"] != "tu_1" || res["content"] != `{"score":61}` {
		t.Errorf("tool_result = %v", res)
	}
}

func TestAnthropicError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	defer srv.Close()
	r := Runner{Client: &Anthropic{Key: "bad", URL: srv.URL}, Model: "m", MaxTokens: 10, MaxRounds: 1}
	_, err := r.Run(context.Background(), NewConversation("", nil, "hi"), nil)
	if err == nil || !strings.Contains(err.Error(), "authentication_error") {
		t.Fatalf("err = %v", err)
	}
}

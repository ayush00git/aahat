package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// AnthropicURL is the Messages endpoint of the Anthropic API.
const AnthropicURL = "https://api.anthropic.com/v1/messages"

// Anthropic calls Claude through the Anthropic API with an API key. It is a Converser, so the
// assistant and the researcher jobs run on it exactly as they do on Amazon Bedrock: a Converse
// request is translated to a Messages request and the reply translated back.
type Anthropic struct {
	Key    string
	URL    string       // AnthropicURL if empty
	Client *http.Client // a client with a 60 s timeout if nil
}

type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int32              `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicResponse struct {
	Content []anthropicBlock `json:"content"`
	Usage   struct {
		InputTokens  int32 `json:"input_tokens"`
		OutputTokens int32 `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (a *Anthropic) Converse(ctx context.Context, in *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	req, err := anthropicFromConverse(in)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	url := a.URL
	if url == "" {
		url = AnthropicURL
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("content-type", "application/json")
	hreq.Header.Set("x-api-key", a.Key)
	hreq.Header.Set("anthropic-version", "2023-06-01")
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	var out anthropicResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("anthropic: status %d: unreadable response", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK || out.Error != nil {
		if out.Error != nil {
			return nil, fmt.Errorf("anthropic: status %d: %s: %s", resp.StatusCode, out.Error.Type, out.Error.Message)
		}
		return nil, fmt.Errorf("anthropic: status %d", resp.StatusCode)
	}

	msg := types.Message{Role: types.ConversationRoleAssistant}
	for _, b := range out.Content {
		switch b.Type {
		case "text":
			msg.Content = append(msg.Content, &types.ContentBlockMemberText{Value: b.Text})
		case "tool_use":
			args := map[string]any{}
			if len(b.Input) > 0 {
				_ = json.Unmarshal(b.Input, &args)
			}
			msg.Content = append(msg.Content, &types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
				ToolUseId: aws.String(b.ID), Name: aws.String(b.Name), Input: document.NewLazyDocument(args),
			}})
		}
	}
	return &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{Value: msg},
		Usage:  &types.TokenUsage{InputTokens: aws.Int32(out.Usage.InputTokens), OutputTokens: aws.Int32(out.Usage.OutputTokens)},
	}, nil
}

// anthropicFromConverse translates the parts of a Converse request this package sends.
func anthropicFromConverse(in *bedrockruntime.ConverseInput) (*anthropicRequest, error) {
	req := &anthropicRequest{Model: aws.ToString(in.ModelId), MaxTokens: 1024}
	if in.InferenceConfig != nil && in.InferenceConfig.MaxTokens != nil {
		req.MaxTokens = *in.InferenceConfig.MaxTokens
	}
	for _, s := range in.System {
		if t, ok := s.(*types.SystemContentBlockMemberText); ok {
			if req.System != "" {
				req.System += "\n\n"
			}
			req.System += t.Value
		}
	}
	for _, m := range in.Messages {
		am := anthropicMessage{Role: string(m.Role)}
		for _, block := range m.Content {
			switch b := block.(type) {
			case *types.ContentBlockMemberText:
				am.Content = append(am.Content, anthropicBlock{Type: "text", Text: b.Value})
			case *types.ContentBlockMemberToolUse:
				input := json.RawMessage(`{}`)
				if b.Value.Input != nil {
					raw, err := b.Value.Input.MarshalSmithyDocument()
					if err != nil {
						return nil, fmt.Errorf("anthropic: tool input: %w", err)
					}
					input = raw
				}
				am.Content = append(am.Content, anthropicBlock{
					Type: "tool_use", ID: aws.ToString(b.Value.ToolUseId), Name: aws.ToString(b.Value.Name), Input: input,
				})
			case *types.ContentBlockMemberToolResult:
				text := ""
				for _, c := range b.Value.Content {
					if t, ok := c.(*types.ToolResultContentBlockMemberText); ok {
						text += t.Value
					}
				}
				am.Content = append(am.Content, anthropicBlock{
					Type: "tool_result", ToolUseID: aws.ToString(b.Value.ToolUseId), Content: text,
					IsError: b.Value.Status == types.ToolResultStatusError,
				})
			default:
				return nil, errors.New("anthropic: unsupported content block")
			}
		}
		req.Messages = append(req.Messages, am)
	}
	if in.ToolConfig != nil {
		for _, tool := range in.ToolConfig.Tools {
			spec, ok := tool.(*types.ToolMemberToolSpec)
			if !ok {
				continue
			}
			schema := json.RawMessage(`{"type":"object","properties":{}}`)
			if js, ok := spec.Value.InputSchema.(*types.ToolInputSchemaMemberJson); ok && js.Value != nil {
				raw, err := js.Value.MarshalSmithyDocument()
				if err != nil {
					return nil, fmt.Errorf("anthropic: tool schema: %w", err)
				}
				schema = raw
			}
			req.Tools = append(req.Tools, anthropicTool{
				Name: aws.ToString(spec.Value.Name), Description: aws.ToString(spec.Value.Description), InputSchema: schema,
			})
		}
	}
	return req, nil
}

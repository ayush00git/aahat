// Package assistant answers villagers' questions in plain Hindi (or English) with Claude on
// Amazon Bedrock. The model never supplies a number itself: it reads the same API data the apps
// show through tools, and every number in its answer is checked against what the tools returned.
package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// Converser is the part of the Bedrock runtime client used here (a fake in tests).
type Converser interface {
	Converse(ctx context.Context, in *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

// Tool is a function the model may call. Schema is a JSON Schema object for its arguments.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
}

// Call records one tool call of a conversation: what was asked and the text the model got back.
type Call struct {
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
	Result string         `json:"-"`
	Failed bool           `json:"-"`
}

// Exec runs one tool call and returns the text shown to the model. An error is reported to the
// model as a failed call (it can then say the data is missing) and does not end the conversation.
type Exec func(ctx context.Context, name string, args map[string]any) (string, error)

// ErrTooManyRounds means the model kept calling tools past Runner.MaxRounds.
var ErrTooManyRounds = errors.New("assistant: tool round limit reached")

// Runner drives a Converse tool-use conversation.
type Runner struct {
	Client    Converser
	Model     string // model id or inference profile id/ARN
	MaxTokens int32
	MaxRounds int // tool rounds allowed before giving up
}

// Conversation is the state of one exchange, kept so a follow-up turn (a correction) can be added.
type Conversation struct {
	System   string
	Tools    []Tool
	Messages []types.Message
	Calls    []Call
	rounds   int
}

// NewConversation starts a conversation with one user message.
func NewConversation(system string, tools []Tool, user string) *Conversation {
	c := &Conversation{System: system, Tools: tools}
	c.Say(user)
	return c
}

// Say appends a user turn.
func (c *Conversation) Say(text string) {
	c.Messages = append(c.Messages, types.Message{
		Role:    types.ConversationRoleUser,
		Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: text}},
	})
}

// Run calls the model until it stops asking for tools, running each tool call through exec, and
// returns the model's final text. The conversation keeps every message and tool call.
func (r Runner) Run(ctx context.Context, c *Conversation, exec Exec) (string, error) {
	for {
		in := &bedrockruntime.ConverseInput{
			ModelId:         aws.String(r.Model),
			Messages:        c.Messages,
			InferenceConfig: &types.InferenceConfiguration{MaxTokens: aws.Int32(r.MaxTokens)},
		}
		if c.System != "" {
			in.System = []types.SystemContentBlock{&types.SystemContentBlockMemberText{Value: c.System}}
		}
		if len(c.Tools) > 0 {
			in.ToolConfig = toolConfig(c.Tools)
		}
		out, err := r.Client.Converse(ctx, in)
		if err != nil {
			return "", fmt.Errorf("bedrock converse: %w", err)
		}
		msg, ok := out.Output.(*types.ConverseOutputMemberMessage)
		if !ok {
			return "", errors.New("bedrock converse: no message in the response")
		}
		c.Messages = append(c.Messages, msg.Value)

		var text strings.Builder
		var results []types.ContentBlock
		for _, block := range msg.Value.Content {
			switch b := block.(type) {
			case *types.ContentBlockMemberText:
				text.WriteString(b.Value)
			case *types.ContentBlockMemberToolUse:
				results = append(results, c.runTool(ctx, b.Value, exec))
			}
		}
		if len(results) == 0 {
			return strings.TrimSpace(text.String()), nil
		}
		c.Messages = append(c.Messages, types.Message{Role: types.ConversationRoleUser, Content: results})
		c.rounds++
		if c.rounds >= r.MaxRounds {
			return "", ErrTooManyRounds
		}
	}
}

func (c *Conversation) runTool(ctx context.Context, use types.ToolUseBlock, exec Exec) types.ContentBlock {
	name := aws.ToString(use.Name)
	args := map[string]any{}
	if use.Input != nil {
		// Malformed input leaves args empty; the tool then reports the missing argument.
		_ = use.Input.UnmarshalSmithyDocument(&args)
	}
	args = plainJSON(args)
	call := Call{Tool: name, Args: args}
	status := types.ToolResultStatusSuccess
	result, err := exec(ctx, name, args)
	if err != nil {
		call.Failed = true
		status = types.ToolResultStatusError
		result = "error: " + err.Error()
	}
	call.Result = result
	c.Calls = append(c.Calls, call)
	return &types.ContentBlockMemberToolResult{Value: types.ToolResultBlock{
		ToolUseId: use.ToolUseId,
		Status:    status,
		Content:   []types.ToolResultContentBlock{&types.ToolResultContentBlockMemberText{Value: result}},
	}}
}

// plainJSON turns the document decoder's number types into what encoding/json would give.
func plainJSON(args map[string]any) map[string]any {
	b, err := json.Marshal(args)
	if err != nil {
		return args
	}
	out := map[string]any{}
	if json.Unmarshal(b, &out) != nil {
		return args
	}
	return out
}

func toolConfig(tools []Tool) *types.ToolConfiguration {
	cfg := &types.ToolConfiguration{}
	for _, t := range tools {
		schema := t.Schema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		cfg.Tools = append(cfg.Tools, &types.ToolMemberToolSpec{Value: types.ToolSpecification{
			Name:        aws.String(t.Name),
			Description: aws.String(t.Description),
			InputSchema: &types.ToolInputSchemaMemberJson{Value: document.NewLazyDocument(schema)},
		}})
	}
	return cfg
}

// StringArg reads a string argument of a tool call ("" if absent or not a string).
func StringArg(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return strings.TrimSpace(s)
}

package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/Nuu-maan/cogent/internal/llm"
)

func TestToWireMessagesMergesToolResults(t *testing.T) {
	// One assistant turn with two tool calls, followed by their two results.
	// The results must collapse into a single user turn with two blocks so the
	// API's user/assistant alternation holds.
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "do it"},
		{Role: llm.RoleAssistant, Content: "on it", ToolCalls: []llm.ToolCall{
			{ID: "a", Name: "read_file", Args: json.RawMessage(`{"path":"x"}`)},
			{ID: "b", Name: "list_dir", Args: json.RawMessage(`{}`)},
		}},
		{Role: llm.RoleTool, ToolCallID: "a", ToolName: "read_file", Content: "x contents"},
		{Role: llm.RoleTool, ToolCallID: "b", ToolName: "list_dir", Content: "x\ny"},
	}

	out := toWireMessages(history)

	if len(out) != 3 {
		t.Fatalf("expected 3 wire messages (user, assistant, user), got %d", len(out))
	}
	if out[0].Role != "user" || out[1].Role != "assistant" || out[2].Role != "user" {
		t.Fatalf("unexpected role sequence: %s, %s, %s", out[0].Role, out[1].Role, out[2].Role)
	}

	// Assistant turn: one text block + two tool_use blocks.
	asst := out[1].Content
	if len(asst) != 3 || asst[0].Type != "text" || asst[1].Type != "tool_use" || asst[2].Type != "tool_use" {
		t.Fatalf("assistant blocks wrong: %+v", asst)
	}

	// The two tool results must be merged into the final user turn.
	results := out[2].Content
	if len(results) != 2 || results[0].Type != "tool_result" || results[1].Type != "tool_result" {
		t.Fatalf("tool results not merged into one user turn: %+v", results)
	}
	if results[0].ToolUseID != "a" || results[1].ToolUseID != "b" {
		t.Fatalf("tool_use_id mismatch: %+v", results)
	}
}

func TestToWireMessagesEmptyAssistantTextOmitted(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "a", Name: "list_dir", Args: json.RawMessage(`{}`)},
		}},
	}
	out := toWireMessages(history)
	if len(out) != 1 || len(out[0].Content) != 1 || out[0].Content[0].Type != "tool_use" {
		t.Fatalf("expected a single tool_use block with no empty text block, got %+v", out)
	}
}

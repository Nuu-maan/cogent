package agent

import "encoding/json"

// UI is the agent's view onto the outside world. Keeping it an interface means
// the loop has no opinion about terminals: the CLI implements it for a human,
// tests implement it to assert on a run, and a future TUI or web frontend can
// implement it without the loop changing.
type UI interface {
	// AssistantText receives streamed assistant text as it arrives.
	AssistantText(chunk string)
	// AssistantDone marks the end of an assistant message (e.g. to flush a line).
	AssistantDone()
	// ToolStart announces a tool invocation about to run.
	ToolStart(name string, args json.RawMessage)
	// ToolEnd reports a tool's result and whether it errored.
	ToolEnd(name, result string, isErr bool)
	// Notice surfaces an out-of-band system message (compaction, warnings).
	Notice(msg string)
}

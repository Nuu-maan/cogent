package llm

import (
	"context"
	"strings"
)

// Complete runs a single non-interactive request and returns the full assistant
// text, ignoring any tool calls. It is a convenience wrapper over Stream for
// callers — such as summarizers — that want one string, not a live stream.
func Complete(ctx context.Context, p Provider, req Request) (string, *Usage, error) {
	events, err := p.Stream(ctx, req)
	if err != nil {
		return "", nil, err
	}
	var sb strings.Builder
	var usage *Usage
	for ev := range events {
		switch ev.Kind {
		case EventText:
			sb.WriteString(ev.Text)
		case EventDone:
			usage = ev.Usage
		case EventError:
			return sb.String(), usage, ev.Err
		}
	}
	return sb.String(), usage, nil
}

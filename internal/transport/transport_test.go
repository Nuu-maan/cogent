package transport

import (
	"strings"
	"testing"
)

func TestScanSSEParsesEventsAndData(t *testing.T) {
	// Mixed stream: a named event with data, a bare data line (OpenAI style),
	// a comment heartbeat, and a multi-line data payload.
	stream := strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start"}`,
		"",
		`data: {"type":"chunk"}`,
		"",
		": heartbeat",
		"data: line1",
		"data: line2",
		"",
		"data: [DONE]",
		"",
	}, "\n")

	var got []Frame
	err := ScanSSE(strings.NewReader(stream), func(f Frame) error {
		got = append(got, f)
		return nil
	})
	if err != nil {
		t.Fatalf("ScanSSE: %v", err)
	}

	want := []Frame{
		{Event: "message_start", Data: `{"type":"message_start"}`},
		{Event: "", Data: `{"type":"chunk"}`},
		{Event: "", Data: "line1\nline2"},
		{Event: "", Data: "[DONE]"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d frames, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("frame %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

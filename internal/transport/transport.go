// Package transport holds HTTP plumbing shared by every llm backend: a
// configured client and a Server-Sent Events reader. Keeping it here means a new
// backend is just request-building plus response-mapping — never socket code.
package transport

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ReadError formats a non-2xx response into a concise, log-safe message,
// including a bounded snippet of the body to aid debugging.
func ReadError(resp *http.Response) string {
	const max = 2 << 10 // 2 KiB is plenty to identify an API error.
	b, _ := io.ReadAll(io.LimitReader(resp.Body, max))
	body := strings.TrimSpace(string(b))
	if body == "" {
		return fmt.Sprintf("http %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return fmt.Sprintf("http %d: %s", resp.StatusCode, body)
}

// NewClient returns an *http.Client with a sane streaming-friendly timeout.
// A zero or negative timeoutSeconds disables the timeout, which is appropriate
// for long-lived streaming responses guarded by a context instead.
func NewClient(timeoutSeconds int) *http.Client {
	var d time.Duration
	if timeoutSeconds > 0 {
		d = time.Duration(timeoutSeconds) * time.Second
	}
	return &http.Client{Timeout: d}
}

// Frame is one Server-Sent Event. Event is the optional "event:" name (empty for
// APIs like OpenAI that omit it); Data is the concatenated "data:" payload.
type Frame struct {
	Event string
	Data  string
}

// ScanSSE parses an SSE stream, invoking fn once per dispatched frame. It returns
// when the stream ends, fn returns an error, or a read error occurs. The
// sentinel payload "[DONE]" is passed through to fn unchanged so callers decide
// what it means.
func ScanSSE(r io.Reader, fn func(Frame) error) error {
	sc := bufio.NewScanner(r)
	// Tool-call argument frames can be large; raise the line cap well above the
	// 64KiB default to avoid bufio.ErrTooLong on dense streams.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var event string
	var data strings.Builder

	dispatch := func() error {
		if data.Len() == 0 && event == "" {
			return nil
		}
		err := fn(Frame{Event: event, Data: data.String()})
		event = ""
		data.Reset()
		return err
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			// Blank line terminates an event.
			if err := dispatch(); err != nil {
				return err
			}
		case strings.HasPrefix(line, ":"):
			// Comment / heartbeat; ignore.
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(line[len("data:"):]))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	// Flush a trailing event with no terminating blank line.
	return dispatch()
}

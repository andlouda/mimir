package otlp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Record is one flattened log record.
type Record struct {
	// Time is the record's own timestamp, Observed the exporter's; Time is
	// zero when the exporter sent none.
	Time     time.Time
	Observed time.Time
	// Name is the event name: the "event.name" attribute, else the string
	// body. Claude Code uses "claude_code.<event>" in the body and the
	// short form in the attribute; Name is always the long form.
	Name     string
	Body     string
	Severity string
	Scope    string
	Attrs    map[string]any
	Resource map[string]any
}

// Span is one flattened span.
type Span struct {
	TraceID       string
	SpanID        string
	ParentSpanID  string
	Name          string
	Start, End    time.Time
	Attrs         map[string]any
	Resource      map[string]any
	StatusCode    int
	StatusMessage string
}

// ParseLogs decodes an ExportLogsServiceRequest in JSON.
func ParseLogs(data []byte) ([]Record, error) {
	var req logsRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("otlp logs: %w", err)
	}
	var out []Record
	for _, rl := range req.ResourceLogs {
		res := attrsToMap(rl.Resource.Attributes)
		for _, sl := range rl.ScopeLogs {
			for _, lr := range sl.LogRecords {
				rec := Record{
					Time:     lr.TimeUnixNano.Time(),
					Observed: lr.ObservedTimeUnixNano.Time(),
					Severity: lr.SeverityText,
					Scope:    sl.Scope.Name,
					Attrs:    attrsToMap(lr.Attributes),
					Resource: res,
				}
				if s, ok := lr.Body.Go().(string); ok {
					rec.Body = s
				}
				rec.Name = eventName(rec.Body, rec.Attrs)
				out = append(out, rec)
			}
		}
	}
	return out, nil
}

// eventName prefers the event.name attribute (normalised to the long
// "claude_code.<name>" form when the body shows that prefix), else the body.
func eventName(body string, attrs map[string]any) string {
	name, _ := attrs["event.name"].(string)
	name = strings.TrimSpace(name)
	body = strings.TrimSpace(body)
	switch {
	case name == "" && body == "":
		return ""
	case name == "":
		return body
	case strings.HasPrefix(name, "claude_code."):
		return name
	case strings.HasPrefix(body, "claude_code.") && body == "claude_code."+name:
		return body
	case strings.HasPrefix(body, "claude_code."):
		return body
	default:
		return name
	}
}

// ParseSpans decodes an ExportTraceServiceRequest in JSON.
func ParseSpans(data []byte) ([]Span, error) {
	var req tracesRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("otlp traces: %w", err)
	}
	var out []Span
	for _, rs := range req.ResourceSpans {
		res := attrsToMap(rs.Resource.Attributes)
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				out = append(out, Span{
					TraceID:       sp.TraceID,
					SpanID:        sp.SpanID,
					ParentSpanID:  sp.ParentSpanID,
					Name:          sp.Name,
					Start:         sp.StartTimeUnixNano.Time(),
					End:           sp.EndTimeUnixNano.Time(),
					Attrs:         attrsToMap(sp.Attributes),
					Resource:      res,
					StatusCode:    sp.Status.Code,
					StatusMessage: sp.Status.Message,
				})
			}
		}
	}
	return out, nil
}

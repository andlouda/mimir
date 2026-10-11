// Package otlp receives OpenTelemetry data over OTLP/HTTP in its JSON
// encoding (what Claude Code sends with OTEL_EXPORTER_OTLP_PROTOCOL=http/json)
// and flattens it into plain records. Only the standard library is used:
// no protobuf, no gRPC, no collector.
package otlp

import (
	"encoding/json"
	"strconv"
	"time"
)

// i64 accepts the two spellings of a 64-bit integer in OTLP JSON: proto3's
// JSON mapping renders int64/uint64/fixed64 as strings ("1718..."), while
// some exporters send plain numbers.
type i64 int64

func (v *i64) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*v = 0
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*v = 0
			return nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			// uint64 beyond int64 range: clamp rather than fail the batch.
			if u, uerr := strconv.ParseUint(s, 10, 64); uerr == nil {
				*v = i64(int64(u))
				return nil
			}
			return err
		}
		*v = i64(n)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*v = i64(int64(f))
	return nil
}

func (v i64) Time() time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(v)).UTC()
}

// anyValue is OTLP's AnyValue.
type anyValue struct {
	StringValue *string      `json:"stringValue"`
	IntValue    *i64         `json:"intValue"`
	DoubleValue *float64     `json:"doubleValue"`
	BoolValue   *bool        `json:"boolValue"`
	BytesValue  *string      `json:"bytesValue"`
	ArrayValue  *arrayValue  `json:"arrayValue"`
	KvlistValue *kvlistValue `json:"kvlistValue"`
}

type arrayValue struct {
	Values []anyValue `json:"values"`
}

type kvlistValue struct {
	Values []keyValue `json:"values"`
}

type keyValue struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

// Go renders the value as the natural Go type: string, int64, float64,
// bool, []any or map[string]any; nil for an empty value.
func (v anyValue) Go() any {
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case v.IntValue != nil:
		return int64(*v.IntValue)
	case v.DoubleValue != nil:
		return *v.DoubleValue
	case v.BoolValue != nil:
		return *v.BoolValue
	case v.BytesValue != nil:
		return *v.BytesValue
	case v.ArrayValue != nil:
		out := make([]any, 0, len(v.ArrayValue.Values))
		for _, x := range v.ArrayValue.Values {
			out = append(out, x.Go())
		}
		return out
	case v.KvlistValue != nil:
		return attrsToMap(v.KvlistValue.Values)
	default:
		return nil
	}
}

func attrsToMap(kvs []keyValue) map[string]any {
	if len(kvs) == 0 {
		return nil
	}
	out := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		if kv.Key == "" {
			continue
		}
		out[kv.Key] = kv.Value.Go()
	}
	return out
}

type resource struct {
	Attributes []keyValue `json:"attributes"`
}

type scope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Logs.

type logsRequest struct {
	ResourceLogs []resourceLogs `json:"resourceLogs"`
}

type resourceLogs struct {
	Resource  resource    `json:"resource"`
	ScopeLogs []scopeLogs `json:"scopeLogs"`
}

type scopeLogs struct {
	Scope      scope       `json:"scope"`
	LogRecords []logRecord `json:"logRecords"`
}

type logRecord struct {
	TimeUnixNano         i64        `json:"timeUnixNano"`
	ObservedTimeUnixNano i64        `json:"observedTimeUnixNano"`
	SeverityText         string     `json:"severityText"`
	Body                 anyValue   `json:"body"`
	Attributes           []keyValue `json:"attributes"`
}

// Traces.

type tracesRequest struct {
	ResourceSpans []resourceSpans `json:"resourceSpans"`
}

type resourceSpans struct {
	Resource   resource     `json:"resource"`
	ScopeSpans []scopeSpans `json:"scopeSpans"`
}

type scopeSpans struct {
	Scope scope  `json:"scope"`
	Spans []span `json:"spans"`
}

type span struct {
	TraceID           string     `json:"traceId"`
	SpanID            string     `json:"spanId"`
	ParentSpanID      string     `json:"parentSpanId"`
	Name              string     `json:"name"`
	Kind              int        `json:"kind"`
	StartTimeUnixNano i64        `json:"startTimeUnixNano"`
	EndTimeUnixNano   i64        `json:"endTimeUnixNano"`
	Attributes        []keyValue `json:"attributes"`
	Status            struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
}

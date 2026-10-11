package otlp

import (
	"testing"
	"time"
)

const sampleLogs = `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"claude-code"}},{"key":"service.version","value":{"stringValue":"2.1.0"}}]},
"scopeLogs":[{"scope":{"name":"com.anthropic.claude_code.events"},"logRecords":[
{"timeUnixNano":"1760179321000000000","observedTimeUnixNano":1760179321500000000,"severityText":"INFO","body":{"stringValue":"claude_code.tool_result"},
 "attributes":[{"key":"event.name","value":{"stringValue":"tool_result"}},{"key":"session.id","value":{"stringValue":"sess-1"}},{"key":"tool_name","value":{"stringValue":"Bash"}},{"key":"tool_use_id","value":{"stringValue":"toolu_1"}},{"key":"duration_ms","value":{"intValue":"1234"}},{"key":"success","value":{"stringValue":"true"}},{"key":"tool_input_size_bytes","value":{"intValue":42}},{"key":"cost","value":{"doubleValue":0.5}},{"key":"flag","value":{"boolValue":true}},
   {"key":"nested","value":{"kvlistValue":{"values":[{"key":"a","value":{"arrayValue":{"values":[{"stringValue":"x"},{"intValue":"7"}]}}}]}}}]},
{"body":{"stringValue":"claude_code.api_request"},"attributes":[{"key":"session.id","value":{"stringValue":"sess-1"}},{"key":"model","value":{"stringValue":"claude-x"}}]},
{"attributes":[{"key":"event.name","value":{"stringValue":"user_prompt"}}]}
]}]}]}`

func TestParseLogsFlattensRecords(t *testing.T) {
	recs, err := ParseLogs([]byte(sampleLogs))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("records: %d", len(recs))
	}
	r := recs[0]
	if r.Name != "claude_code.tool_result" || r.Body != "claude_code.tool_result" || r.Severity != "INFO" || r.Scope != "com.anthropic.claude_code.events" {
		t.Fatalf("record head: %+v", r)
	}
	if !r.Time.Equal(time.Unix(1760179321, 0)) || !r.Observed.Equal(time.Unix(1760179321, 500000000)) {
		t.Fatalf("times: %v %v", r.Time, r.Observed)
	}
	if r.Attrs["duration_ms"] != int64(1234) || r.Attrs["tool_input_size_bytes"] != int64(42) || r.Attrs["cost"] != 0.5 || r.Attrs["flag"] != true || r.Attrs["success"] != "true" {
		t.Fatalf("attrs: %#v", r.Attrs)
	}
	nested := r.Attrs["nested"].(map[string]any)["a"].([]any)
	if nested[0] != "x" || nested[1] != int64(7) {
		t.Fatalf("nested: %#v", nested)
	}
	if r.Resource["service.name"] != "claude-code" {
		t.Fatalf("resource: %#v", r.Resource)
	}
	if recs[1].Name != "claude_code.api_request" || !recs[1].Time.IsZero() {
		t.Fatalf("body-only record: %+v", recs[1])
	}
	if recs[2].Name != "user_prompt" {
		t.Fatalf("attribute-only record: %+v", recs[2])
	}
	if _, err := ParseLogs([]byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":"abc"}]}]}]}`)); err == nil {
		t.Fatal("non-numeric timestamp must fail")
	}
	if _, err := ParseLogs([]byte(`not json`)); err == nil {
		t.Fatal("invalid json must fail")
	}
}

func TestParseSpans(t *testing.T) {
	data := `{"resourceSpans":[{"resource":{"attributes":[{"key":"session.id","value":{"stringValue":"sess-1"}}]},"scopeSpans":[{"spans":[
	{"traceId":"abc","spanId":"01","parentSpanId":"","name":"claude_code.interaction","startTimeUnixNano":"1760179321000000000","endTimeUnixNano":"1760179323000000000","attributes":[{"key":"interaction.sequence","value":{"intValue":"3"}}],"status":{"code":1}},
	{"traceId":"abc","spanId":"02","parentSpanId":"01","name":"claude_code.tool","startTimeUnixNano":"1760179321500000000","endTimeUnixNano":"1760179322000000000","attributes":[{"key":"tool_name","value":{"stringValue":"Bash"}},{"key":"tool_use_id","value":{"stringValue":"toolu_1"}}]}
	]}]}]}`
	spans, err := ParseSpans([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 || spans[1].ParentSpanID != "01" || spans[1].Attrs["tool_use_id"] != "toolu_1" || spans[0].Resource["session.id"] != "sess-1" {
		t.Fatalf("spans: %+v", spans)
	}
	if spans[0].End.Sub(spans[0].Start) != 2*time.Second || spans[0].StatusCode != 1 {
		t.Fatalf("span timing: %+v", spans[0])
	}
}

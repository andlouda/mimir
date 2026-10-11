package otlp

import (
	"bytes"
	"compress/gzip"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, token string, traces bool) (*Server, *httptest.Server, *[]Record, *[]Span) {
	var recs []Record
	var spans []Span
	s := &Server{Token: token, AcceptTraces: traces, MaxBody: 2048,
		OnLogs:  func(r []Record) { recs = append(recs, r...) },
		OnSpans: func(sp []Span) { spans = append(spans, sp...) },
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts, &recs, &spans
}

func post(t *testing.T, url, body string, headers map[string]string) *http.Response {
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestServerAcceptsLogsRejectsBadRequests(t *testing.T) {
	_, ts, recs, spans := newTestServer(t, "secret", false)
	ok := map[string]string{TokenHeader: "secret"}
	if r := post(t, ts.URL+"/v1/logs", sampleLogs, ok); r.StatusCode != 200 {
		t.Fatalf("logs: %d", r.StatusCode)
	}
	if len(*recs) != 3 {
		t.Fatalf("records delivered: %d", len(*recs))
	}
	if r := post(t, ts.URL+"/v1/logs", sampleLogs, map[string]string{"Authorization": "Bearer secret"}); r.StatusCode != 200 {
		t.Fatalf("bearer form: %d", r.StatusCode)
	}
	if r := post(t, ts.URL+"/v1/logs", sampleLogs, nil); r.StatusCode != 401 {
		t.Fatalf("missing token: %d", r.StatusCode)
	}
	if r := post(t, ts.URL+"/v1/logs", sampleLogs, map[string]string{TokenHeader: "wrong"}); r.StatusCode != 401 {
		t.Fatalf("wrong token: %d", r.StatusCode)
	}
	if r := post(t, ts.URL+"/v1/logs", sampleLogs, map[string]string{TokenHeader: "secret", "Content-Type": "application/x-protobuf"}); r.StatusCode != 415 {
		t.Fatalf("protobuf: %d", r.StatusCode)
	}
	if r := post(t, ts.URL+"/v1/logs", `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":"x"}]}]}]}`, ok); r.StatusCode != 400 {
		t.Fatalf("bad body: %d", r.StatusCode)
	}
	big := `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"` + strings.Repeat("a", 3000) + `"}}]}]}]}`
	if r := post(t, ts.URL+"/v1/logs", big, ok); r.StatusCode != 413 {
		t.Fatalf("too large: %d", r.StatusCode)
	}
	if r := post(t, ts.URL+"/v1/metrics", `{"resourceMetrics":[]}`, ok); r.StatusCode != 200 {
		t.Fatalf("metrics must be acknowledged: %d", r.StatusCode)
	}
	if r := post(t, ts.URL+"/v1/traces", `{"resourceSpans":[{"scopeSpans":[{"spans":[{"name":"x"}]}]}]}`, ok); r.StatusCode != 200 || len(*spans) != 0 {
		t.Fatalf("traces off: %d %d", r.StatusCode, len(*spans))
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/logs", nil)
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 405 {
		t.Fatalf("GET: %d", resp.StatusCode)
	}
}

func TestServerGzipAndTraces(t *testing.T) {
	_, ts, recs, spans := newTestServer(t, "", true)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(sampleLogs))
	_ = gz.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/logs", &buf)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || len(*recs) != 3 {
		t.Fatalf("gzip: %d %d", resp.StatusCode, len(*recs))
	}
	if r := post(t, ts.URL+"/v1/traces", `{"resourceSpans":[{"scopeSpans":[{"spans":[{"name":"claude_code.tool","spanId":"1"}]}]}]}`, nil); r.StatusCode != 200 || len(*spans) != 1 {
		t.Fatalf("traces on: %d %d", r.StatusCode, len(*spans))
	}
}

func TestServerStartBindsLoopbackOnly(t *testing.T) {
	s := &Server{Addr: "0.0.0.0:0"}
	if err := s.Start(); err != ErrNotLoopback {
		t.Fatalf("non-loopback must be refused: %v", err)
	}
	s = &Server{Addr: "127.0.0.1:0"}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	addr := s.ListenAddr()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("addr: %s", addr)
	}
	if r := post(t, "http://"+addr+"/v1/metrics", "{}", nil); r.StatusCode != 200 {
		t.Fatalf("live server: %d", r.StatusCode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil || s.ListenAddr() != "" {
		t.Fatalf("stop: %v %q", err, s.ListenAddr())
	}
}

func TestServerAllowFromPermitsNonLoopbackClients(t *testing.T) {
	_, n, _ := net.ParseCIDR("172.20.0.0/20")
	s := &Server{Token: "secret", AllowFrom: []*net.IPNet{n}}
	if !s.allowed("127.0.0.1") || !s.allowed("172.20.3.7") || s.allowed("172.21.0.1") || s.allowed("203.0.113.9") || s.allowed("nope") {
		t.Fatal("allowed() classification")
	}
	// A non-loopback bind needs both AllowFrom and a token.
	if err := (&Server{Addr: "0.0.0.0:0", AllowFrom: []*net.IPNet{n}}).Start(); err != ErrNotLoopback {
		t.Fatalf("no token must be refused: %v", err)
	}
	if err := (&Server{Addr: "0.0.0.0:0", Token: "secret"}).Start(); err != ErrNotLoopback {
		t.Fatalf("no AllowFrom must be refused: %v", err)
	}
}

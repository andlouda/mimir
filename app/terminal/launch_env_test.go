package terminal

import (
	"strings"
	"testing"
)

func TestLaunchEnvFiltersAndRenders(t *testing.T) {
	SetLaunchEnvProvider(func() []string {
		return []string{"CLAUDE_CODE_ENABLE_TELEMETRY=1", "OTEL_EXPORTER_OTLP_HEADERS=x-mimir-token=ab'cd", "bad name=1", "NOEQUALS", "EVIL=a\nb", "=x"}
	})
	t.Cleanup(func() { SetLaunchEnvProvider(nil) })
	env := launchEnv()
	if len(env) != 2 || env[0] != "CLAUDE_CODE_ENABLE_TELEMETRY=1" || env[1] != "OTEL_EXPORTER_OTLP_HEADERS=x-mimir-token=ab'cd" {
		t.Fatalf("filtered env: %#v", env)
	}
	exports := launchEnvExports(false)
	if !strings.Contains(exports, "export CLAUDE_CODE_ENABLE_TELEMETRY='1'\n") || !strings.Contains(exports, `export OTEL_EXPORTER_OTLP_HEADERS='x-mimir-token=ab'\''cd'`+"\n") {
		t.Fatalf("exports:\n%s", exports)
	}
	if strings.Contains(exports, "EVIL") || strings.Contains(exports, "bad name") {
		t.Fatalf("bad entries leaked:\n%s", exports)
	}
	SetLaunchEnvProvider(nil)
	if launchEnv() != nil || launchEnvExports(false) != "" {
		t.Fatal("no provider must mean no env")
	}
}

func TestRenderExportsWSLProbe(t *testing.T) {
	env := []string{"OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:14318", "OTEL_LOGS_EXPORTER=otlp"}
	plain := RenderExports(env, false)
	if !strings.Contains(plain, "export OTEL_EXPORTER_OTLP_ENDPOINT='http://127.0.0.1:14318'") || strings.Contains(plain, "/dev/tcp") {
		t.Fatalf("plain:\n%s", plain)
	}
	wsl := RenderExports(env, true)
	if !strings.Contains(wsl, "/dev/tcp/127.0.0.1/14318") || !strings.Contains(wsl, "ip route show default") || !strings.Contains(wsl, `export OTEL_EXPORTER_OTLP_ENDPOINT="http://${__mimir_otel_host:-127.0.0.1}:14318"`) || !strings.Contains(wsl, "export OTEL_LOGS_EXPORTER='otlp'") {
		t.Fatalf("wsl:\n%s", wsl)
	}
	// A non-loopback endpoint is left alone even for WSL.
	other := RenderExports([]string{"OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318"}, true)
	if strings.Contains(other, "/dev/tcp") {
		t.Fatalf("other:\n%s", other)
	}
}

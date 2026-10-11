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
	exports := launchEnvExports()
	if !strings.Contains(exports, "export CLAUDE_CODE_ENABLE_TELEMETRY='1'\n") || !strings.Contains(exports, `export OTEL_EXPORTER_OTLP_HEADERS='x-mimir-token=ab'\''cd'`+"\n") {
		t.Fatalf("exports:\n%s", exports)
	}
	if strings.Contains(exports, "EVIL") || strings.Contains(exports, "bad name") {
		t.Fatalf("bad entries leaked:\n%s", exports)
	}
	SetLaunchEnvProvider(nil)
	if launchEnv() != nil || launchEnvExports() != "" {
		t.Fatal("no provider must mean no env")
	}
}

package aiflow

import "testing"

func TestNormalizeDiscoveryOutput(t *testing.T) {
	values := normalizeDiscoveryOutput("api-2\r\napi-1\n\napi-2\n")
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0] != "api-1" || values[1] != "api-2" {
		t.Fatalf("unexpected normalized values: %+v", values)
	}
}

func TestBuildDiscoveryCacheKeyDeterministic(t *testing.T) {
	left := buildDiscoveryCacheKey("discovery:list_k8s_pods", "bash", map[string]string{
		"Pod":       "api",
		"Namespace": "default",
	})
	right := buildDiscoveryCacheKey("discovery:list_k8s_pods", "bash", map[string]string{
		"Namespace": "default",
		"Pod":       "api",
	})
	if left != right {
		t.Fatalf("expected deterministic cache key, got %q vs %q", left, right)
	}
}

func TestDiscoveryCommandValidation(t *testing.T) {
	if _, _, err := discoveryCommand("discovery:list_k8s_pods", "bash", map[string]string{}); err == nil {
		t.Fatalf("expected missing namespace to fail")
	}
	if _, _, err := discoveryCommand("discovery:list_k8s_resources", "bash", map[string]string{
		"Namespace": "default",
	}); err == nil {
		t.Fatalf("expected missing resource type to fail")
	}
}

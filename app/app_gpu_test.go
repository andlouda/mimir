package main

import "testing"

func TestGPUPolicyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	a := &App{}
	if got := a.GetGPUPolicy(); got != GPUPolicyNever {
		t.Fatalf("default must be never, got %q", got)
	}
	if got, err := a.SetGPUPolicy("OnDemand"); err != nil || got != GPUPolicyOnDemand {
		t.Fatalf("set: %v %q", err, got)
	}
	if got := a.GetGPUPolicy(); got != GPUPolicyOnDemand {
		t.Fatalf("persisted policy expected, got %q", got)
	}
	if got, _ := a.SetGPUPolicy("garbage"); got != GPUPolicyNever {
		t.Fatalf("unknown values fall back to never, got %q", got)
	}
}

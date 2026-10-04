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

func TestWaylandFixDecision(t *testing.T) {
	cases := []struct {
		mode    string
		wayland bool
		set     bool
		want    bool
	}{
		{WaylandFixAuto, true, false, true},
		{WaylandFixAuto, false, false, false},
		{WaylandFixOn, false, false, true},
		{WaylandFixOff, true, false, false},
		{WaylandFixAuto, true, true, false}, // the user's own variable wins
	}
	for _, c := range cases {
		if got := shouldDisableDMABUF(c.mode, c.wayland, c.set); got != c.want {
			t.Fatalf("mode=%s wayland=%v set=%v: got %v", c.mode, c.wayland, c.set, got)
		}
	}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	a := &App{}
	if a.GetWaylandFix() != WaylandFixAuto {
		t.Fatalf("default must be auto")
	}
	if got, err := a.SetWaylandFix("ON"); err != nil || got != WaylandFixOn || a.GetWaylandFix() != WaylandFixOn {
		t.Fatalf("set/get: %v %q", err, got)
	}
}

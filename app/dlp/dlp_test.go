package dlp

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestMaskReplacesIdentifiersWithStablePlaceholders(t *testing.T) {
	m := NewMapping()
	in := "ping 10.0.3.17: timeout\nroute 10.0.3.17 via 10.0.3.1 dev eth0\nmail admin@Example.com, ADMIN@example.com"
	got := m.Mask(in)
	want := "ping MIMIR_IP_1: timeout\nroute MIMIR_IP_1 via MIMIR_IP_2 dev eth0\nmail MIMIR_EMAIL_1, MIMIR_EMAIL_1"
	if got != want {
		t.Fatalf("Mask:\n got %q\nwant %q", got, want)
	}
	if counts := m.Counts(); !reflect.DeepEqual(counts, map[string]int{"IP": 3, "EMAIL": 2}) {
		t.Fatalf("Counts = %v", counts)
	}
}

func TestMaskKeepsPlaceholdersConsistentAcrossTexts(t *testing.T) {
	m := NewMapping()
	output := m.Mask("server 192.168.1.20 down")
	goal := m.Mask("restart nginx on 192.168.1.20 and check 192.168.1.21")
	if output != "server MIMIR_IP_1 down" || goal != "restart nginx on MIMIR_IP_1 and check MIMIR_IP_2" {
		t.Fatalf("got %q / %q", output, goal)
	}
}

func TestMaskCoversIPv6MACAndCIDR(t *testing.T) {
	m := NewMapping()
	in := "inet6 fe80::1c2b:3aff:fe4d:5e6f/64 scope link\nether 00:1A:2B:3C:4D:5E\ninet 172.16.4.9/24 brd 172.16.4.255\nvia 2001:db8::1 and ::ffff:10.1.2.3"
	got := m.Mask(in)
	for _, leak := range []string{"fe80::1c2b", "00:1A:2B", "172.16.4.9", "172.16.4.255", "2001:db8::1", "10.1.2.3"} {
		if strings.Contains(got, leak) {
			t.Errorf("%q still in masked text: %s", leak, got)
		}
	}
	if !strings.Contains(got, "MIMIR_IP6_1/64") || !strings.Contains(got, "MIMIR_IP_1/24") || !strings.Contains(got, "ether MIMIR_MAC_1") {
		t.Errorf("unexpected masked text: %s", got)
	}
}

func TestMaskLeavesNonIdentifiersAlone(t *testing.T) {
	cases := []string{
		"listening on 127.0.0.1:8080 and 0.0.0.0:443",
		"netmask 255.255.255.0",
		"loopback ::1 and any ::",
		"version 1.2.3.4.5 and 2.10.0.1-beta",
		"std::vector<int> a::b c::",
		"12:34:56 up 3 days",
		"Chrome 120.0.6099.71",
		"t3@pironman:~$ ls",
		"999.1.1.1 is not an address",
		"file_10.0.0.1_backup", // glued to an identifier
	}
	for _, in := range cases {
		m := NewMapping()
		if got := m.Mask(in); got != in {
			t.Errorf("Mask(%q) = %q, want unchanged", in, got)
		}
		if !m.Empty() {
			t.Errorf("Mask(%q) recorded placeholders", in)
		}
	}
}

func TestUnmaskRestoresTheOriginalSpelling(t *testing.T) {
	m := NewMapping()
	masked := m.Mask("hosts 10.0.3.17 and FE80::1 and 00-1A-2B-3C-4D-5E")
	answer := "ssh root@MIMIR_IP_1 && ping6 MIMIR_IP6_1 # MIMIR_MAC_1, again MIMIR_IP_1"
	if strings.Contains(masked, "10.0.3.17") {
		t.Fatalf("not masked: %s", masked)
	}
	got, report := m.Unmask(answer)
	want := "ssh root@10.0.3.17 && ping6 FE80::1 # 00-1A-2B-3C-4D-5E, again 10.0.3.17"
	if got != want {
		t.Fatalf("Unmask:\n got %q\nwant %q", got, want)
	}
	if report.Restored != 4 || len(report.Unknown) != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestUnmaskReportsPlaceholdersItNeverHandedOut(t *testing.T) {
	m := NewMapping()
	m.Mask("host 10.0.3.17")
	got, report := m.Unmask("scp f MIMIR_IP_1:/tmp && ssh MIMIR_IP_7 && ssh MIMIR_IP_7 && mail MIMIR_EMAIL_2")
	if got != "scp f 10.0.3.17:/tmp && ssh MIMIR_IP_7 && ssh MIMIR_IP_7 && mail MIMIR_EMAIL_2" {
		t.Fatalf("Unmask = %q", got)
	}
	if !reflect.DeepEqual(report.Unknown, []string{"MIMIR_EMAIL_2", "MIMIR_IP_7"}) || report.Restored != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestUnmaskDoesNotConfusePlaceholderNumbers(t *testing.T) {
	m := NewMapping()
	var b strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&b, "10.0.0.%d ", i)
	}
	m.Mask(b.String())
	got, report := m.Unmask("MIMIR_IP_1 MIMIR_IP_11 MIMIR_IP_12")
	if got != "10.0.0.1 10.0.0.11 10.0.0.12" || report.Restored != 3 {
		t.Fatalf("Unmask = %q, report %+v", got, report)
	}
}

func TestRoundTripIsLossless(t *testing.T) {
	in := "eth0: 10.20.30.40/24 fe80::dead:beef/64 aa:bb:cc:dd:ee:ff ops@corp.example 10.20.30.40"
	m := NewMapping()
	got, _ := m.Unmask(m.Mask(in))
	if got != in {
		t.Fatalf("round trip:\n got %q\nwant %q", got, in)
	}
}

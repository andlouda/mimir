package ssh

import "testing"

func TestValidateProfileRequiresHostAndUsername(t *testing.T) {
	p := Profile{Host: "", Username: "user"}
	if err := validateProfile(&p); err == nil {
		t.Fatal("expected error for empty host")
	}
	p = Profile{Host: "example.com", Username: ""}
	if err := validateProfile(&p); err == nil {
		t.Fatal("expected error for empty username")
	}
}

func TestValidateProfilePortRange(t *testing.T) {
	p := Profile{Host: "example.com", Username: "user", Port: 0}
	if err := validateProfile(&p); err != nil {
		t.Fatalf("port 0 should default to 22: %v", err)
	}
	if p.Port != 22 {
		t.Fatalf("expected port 22, got %d", p.Port)
	}

	p = Profile{Host: "example.com", Username: "user", Port: 99999}
	if err := validateProfile(&p); err == nil {
		t.Fatal("expected error for port 99999")
	}

	p = Profile{Host: "example.com", Username: "user", Port: -1}
	if err := validateProfile(&p); err == nil {
		t.Fatal("expected error for negative port")
	}
}

func TestValidateProfileRejectsNullBytes(t *testing.T) {
	p := Profile{Host: "example\x00.com", Username: "user"}
	if err := validateProfile(&p); err == nil {
		t.Fatal("expected error for null byte in host")
	}

	p = Profile{Host: "example.com", Username: "user\x00name"}
	if err := validateProfile(&p); err == nil {
		t.Fatal("expected error for null byte in username")
	}
}

func TestValidateProfileLengthLimits(t *testing.T) {
	longHost := make([]byte, 300)
	for i := range longHost {
		longHost[i] = 'a'
	}
	p := Profile{Host: string(longHost), Username: "user"}
	if err := validateProfile(&p); err == nil {
		t.Fatal("expected error for host exceeding max length")
	}
}

func TestValidateProfileJumpHostRequired(t *testing.T) {
	p := Profile{
		Host:            "example.com",
		Username:        "user",
		JumpHostEnabled: true,
		JumpHost:        "",
	}
	if err := validateProfile(&p); err == nil {
		t.Fatal("expected error for enabled jump host with empty host")
	}

	p.JumpHost = "jump.example.com"
	if err := validateProfile(&p); err != nil {
		t.Fatalf("valid jump host config should pass: %v", err)
	}
}

func TestValidateProfileKeyPathLength(t *testing.T) {
	longPath := make([]byte, 2000)
	for i := range longPath {
		longPath[i] = '/'
	}
	p := Profile{
		Host:       "example.com",
		Username:   "user",
		AuthMethod: "key",
		KeyPath:    string(longPath),
	}
	if err := validateProfile(&p); err == nil {
		t.Fatal("expected error for key path exceeding max length")
	}
}

func TestValidateProfileAcceptsValid(t *testing.T) {
	p := Profile{
		Host:     "example.com",
		Username: "deploy",
		Port:     22,
	}
	if err := validateProfile(&p); err != nil {
		t.Fatalf("valid profile should pass: %v", err)
	}
}

package desktop

import (
	"fmt"
	"strings"
	"testing"
)

func TestDesktopTemplateUsesThemeIconNameAndWMClass(t *testing.T) {
	content := fmt.Sprintf(desktopTemplate, "/opt/mimir/mimir")

	for _, want := range []string{
		"Icon=mimir",
		"StartupWMClass=mimir",
		"StartupNotify=true",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("desktop entry missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "Icon=/") {
		t.Fatalf("desktop entry should use theme icon name, not absolute icon path:\n%s", content)
	}
}

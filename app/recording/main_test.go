package recording

import (
	"testing"

	"mimir/internal/testenv"
)

// TestMain keeps the tests of this package off the real user data directory.
func TestMain(m *testing.M) { testenv.Main(m) }

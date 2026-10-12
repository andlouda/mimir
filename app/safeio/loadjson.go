package safeio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// ErrCorrupt marks a JSON file that exists but does not parse. LoadJSON
// has already moved such a file aside when it returns this.
var ErrCorrupt = errors.New("corrupt json file")

// LoadJSON reads path into v. A missing file leaves v untouched and
// returns nil, so callers start from their defaults. A file that does not
// parse is moved to <path>.corrupt-<timestamp> and ErrCorrupt (wrapped
// with both paths) is returned: the broken file stays for recovery, and
// the caller's next save cannot overwrite it. Read errors other than
// "missing" are returned as they are.
func LoadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		aside := fmt.Sprintf("%s.corrupt-%s", path, time.Now().UTC().Format("20060102-150405"))
		if mvErr := renameWithRetry(path, aside); mvErr != nil {
			return fmt.Errorf("%w: %s (%v); could not move it aside: %v", ErrCorrupt, path, err, mvErr)
		}
		return fmt.Errorf("%w: %s (%v); moved to %s", ErrCorrupt, path, err, aside)
	}
	return nil
}

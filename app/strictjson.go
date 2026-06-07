package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func strictUnmarshalJSON(raw string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("strict json decode failed: %w", err)
	}
	if decoder.More() {
		return fmt.Errorf("strict json decode failed: trailing data")
	}
	return nil
}

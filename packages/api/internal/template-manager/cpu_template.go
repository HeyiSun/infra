package template_manager

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// normalizeCPUTemplate normalizes JSON object formatting while preserving values
// and modifier order. An absent or empty configuration is stored as SQL NULL.
func normalizeCPUTemplate(raw string) (json.RawMessage, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	var config map[string]any
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("invalid CPU template JSON object: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid CPU template JSON: trailing data")
	}
	if len(config) == 0 {
		return nil, nil
	}

	// encoding/json sorts object keys recursively. Arrays keep their order.
	normalized, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("normalize CPU template JSON: %w", err)
	}

	return normalized, nil
}

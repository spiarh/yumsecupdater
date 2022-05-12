package main

import (
	"fmt"
	"strings"
	"time"
)

// parseDurationString parses the cli values duration string.
func parseDurationString(duration string) (time.Duration, error) {
	var d time.Duration
	var err error
	d, err = time.ParseDuration(duration)
	if err != nil {
		return d, fmt.Errorf("update interval invalid: %s", duration)
	}

	return d, nil
}

// parseCommaSeparatedFlagValues parses the cli values that are separated
// with comma and returns a slice from it.
func parseCommaSeparatedFlagValues(value string) []string {
	if value != "" {
		return strings.Split(value, ",")
	}
	return []string{}
}

// validateSeverity checks if a severity is valid.
func validateSeverity(s string) error {
	var allowedSeverities = map[string]struct{}{
		"Low":       {},
		"Moderate":  {},
		"Medium":    {},
		"Important": {},
		"Critical":  {},
	}

	if _, ok := allowedSeverities[s]; !ok {
		return fmt.Errorf("invalid severity: %s", s)
	}
	return nil
}

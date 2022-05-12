package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/assert"
)

func TestParseDurationString(t *testing.T) {
	var tests = []struct {
		input   string
		wantErr bool
	}{
		{"24h", false},
		{"15m", false},
		{"15", true},
		{"ewqfoiwejf", true},
		{"h", true},
	}

	for _, tt := range tests {
		_, err := parseDurationString(tt.input)
		if tt.wantErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestParseCommaSeparatedFlagValues(t *testing.T) {
	var tests = []struct {
		input string
		want  []string
	}{
		{"arg1,arg2,arg3", []string{"arg1", "arg2", "arg3"}},
		{"arg1arg2arg3", []string{"arg1arg2arg3"}},
		{"", []string{}},
	}

	for _, tt := range tests {
		result := parseCommaSeparatedFlagValues(tt.input)
		assert.Equal(t, result, tt.want)
	}
}

func TestValidateSeverity(t *testing.T) {
	var tests = []struct {
		input   string
		wantErr bool
	}{
		{"Low", false},
		{"low", true},
		{"severe", true},
	}

	for _, tt := range tests {
		err := validateSeverity(tt.input)
		if tt.wantErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}

}

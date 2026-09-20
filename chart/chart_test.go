package chart

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input string
		want  Mode
	}{
		{"light", ModeLight}, {"LIGHT", ModeLight},
		{"dark", ModeDark}, {" Dark ", ModeDark},
	} {
		got, ok := ParseMode(tc.input)
		assert.True(t, ok, tc.input)
		assert.Equal(t, tc.want, got, tc.input)
	}

	// Anything else means "leave the spec's mode alone", not an error.
	for _, bad := range []string{"", "sepia", "auto", "system"} {
		_, ok := ParseMode(bad)
		assert.False(t, ok, bad)
	}
}

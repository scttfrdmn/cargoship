package helpers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchGlobs(t *testing.T) {
	tests := []struct {
		globs []string
		path  string
		want  bool
	}{
		{[]string{"*"}, "foo", true},
		{[]string{"*.out"}, "thing.out", true},
		{[]string{"*.out"}, "thing.in", false},
	}
	for _, tt := range tests {
		got := FilenameMatchesGlobs(tt.path, tt.globs)
		require.Equal(t, tt.want, got)
	}
}

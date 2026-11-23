package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

// TestSyncResultError pins #902's CLI half: `sync` must fail when the pipeline
// returned no error but did not succeed, instead of printing "Sync complete!".
func TestSyncResultError(t *testing.T) {
	chunkErr := errors.New("upload chunk-1: InternalError")
	tests := []struct {
		name    string
		result  *pipeline.Result
		wantErr bool
		want    []string
	}{
		{name: "success", result: &pipeline.Result{Success: true}},
		{name: "nil result", result: nil, wantErr: true, want: []string{"no result"}},
		{name: "failed without errors", result: &pipeline.Result{}, wantErr: true, want: []string{"did not complete", "rerun sync"}},
		{
			name:    "failed chunk",
			result:  &pipeline.Result{Errors: []error{chunkErr, errors.New("second")}},
			wantErr: true,
			want:    []string{"2 error(s)", "rerun sync", "chunk-1"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := syncResultError(tc.result)
			if !tc.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, w := range tc.want {
				assert.Contains(t, err.Error(), w)
			}
			if tc.result != nil && len(tc.result.Errors) > 0 {
				assert.ErrorIs(t, err, chunkErr)
			}
		})
	}
}

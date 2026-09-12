package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// TestWaitForCompletion_CountsSkippedSeparately guards #447-A: a chunk skipped on
// resume (Job.Skipped) must be counted in Result.ChunksSkipped, not
// ChunksUploaded, and TotalChunks is the sum. Pre-fix every job — skipped or not —
// incremented ChunksUploaded and ChunksSkipped stayed 0.
func TestWaitForCompletion_CountsSkippedSeparately(t *testing.T) {
	p := &Pipeline{
		config:     &PipelineConfig{}, // UseRealS3 false, no manifest upload
		resultChan: make(chan *Job, 4),
		progress:   &ProgressTracker{progress: Progress{StartTime: time.Now()}},
	}

	p.resultChan <- &Job{Chunk: chunking.Chunk{FileCount: 1}}                // uploaded
	p.resultChan <- &Job{Chunk: chunking.Chunk{FileCount: 1}, Skipped: true} // skipped
	p.resultChan <- &Job{Chunk: chunking.Chunk{FileCount: 1}, Skipped: true} // skipped
	close(p.resultChan)

	res := p.waitForCompletion(context.Background())

	assert.Equal(t, 1, res.ChunksUploaded, "only the non-skipped chunk is an upload")
	assert.Equal(t, 2, res.ChunksSkipped, "skipped chunks are counted as skipped")
	assert.Equal(t, 3, res.TotalChunks, "TotalChunks = uploaded + skipped")
}

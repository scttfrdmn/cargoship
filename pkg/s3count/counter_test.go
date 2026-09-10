package s3count

import (
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCounterMechanics(t *testing.T) {
	c := New()
	assert.Equal(t, 0, c.Total())

	// Concurrent increments from many goroutines (mirrors the parallel uploader).
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.inc("PutObject") }()
	}
	wg.Wait()
	c.inc("GetObject")
	c.inc("")

	counts := c.Counts()
	assert.Equal(t, 50, counts["PutObject"])
	assert.Equal(t, 1, counts["GetObject"])
	assert.Equal(t, 1, counts["Unknown"], "empty op name is bucketed as Unknown")
	assert.Equal(t, 52, c.Total())
	assert.Equal(t, []string{"GetObject", "PutObject", "Unknown"}, c.Names())

	c.Reset()
	assert.Equal(t, 0, c.Total())
	assert.Empty(t, c.Counts())
}

// TestInstrumentRegistersMiddleware confirms Instrument appends an APIOption
// that registers the counting middleware in the Initialize step. (End-to-end
// operation counting is validated by the harness runner against the emulator.)
func TestInstrumentRegistersMiddleware(t *testing.T) {
	c := New()
	cfg := &aws.Config{}
	c.Instrument(cfg)
	require.Len(t, cfg.APIOptions, 1, "Instrument must append one APIOption")

	stack := middleware.NewStack("test", smithyhttp.NewStackRequest)
	require.NoError(t, cfg.APIOptions[0](stack))
	assert.Contains(t, stack.Initialize.List(), "cargoship-s3count",
		"the counting middleware must be registered in the Initialize step")
}

package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnalyze(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(b)
	cmd.SetArgs([]string{"analyze", "../../../pkg/testdata/fake-dir"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, `largestfilesize: 60
largestfilesizehr: 60 B
filecount: 7
averagefilesize: 22
averagefilesizehr: 22 B
totalfilesize: 157
totalfilesizehr: 157 B
`, b.String())
}

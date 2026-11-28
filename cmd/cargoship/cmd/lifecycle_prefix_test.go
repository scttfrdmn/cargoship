package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/aws/lifecycle"
)

// #993: templates take the user's upload prefix, and the flags are checked
// before any AWS call.

func TestLifecycleTemplate_PrefixValidation(t *testing.T) {
	tests := []struct {
		name    string
		flags   map[string]string
		wantErr string
	}{
		{
			name:    "template without --prefix",
			flags:   map[string]string{"bucket": "b", "template": "archive-optimization"},
			wantErr: "--prefix is required with --template",
		},
		{
			name:    "compliance over the whole bucket",
			flags:   map[string]string{"bucket": "b", "template": "compliance-retention", "prefix": ""},
			wantErr: "needs the prefix your uploads use",
		},
		{
			name:    "unknown template",
			flags:   map[string]string{"bucket": "b", "template": "nope", "prefix": "backup"},
			wantErr: "not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewLifecycleCmd()
			for k, v := range tt.flags {
				require.NoError(t, cmd.Flags().Set(k, v))
			}
			err := cmd.RunE(cmd, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Equal(t, ExitUsage, ExitCode(err), "a bad invocation exits 2")
		})
	}
}

func TestResolveLifecycleTemplate_UsesPrefix(t *testing.T) {
	cmd := NewLifecycleCmd()
	require.NoError(t, cmd.Flags().Set("prefix", "backup/daily"))
	tmpl, err := resolveLifecycleTemplate(cmd, "archive-optimization", "backup/daily")
	require.NoError(t, err)
	for _, r := range tmpl.Rules {
		assert.Equal(t, "backup/daily/", r.Filter.Prefix, "rule %s", r.ID)
		if len(r.Transitions) > 0 {
			assert.Equal(t, lifecycle.ChunkTagValue, r.Filter.Tags[lifecycle.ChunkTagKey], "transitions only move chunks")
		}
	}

	// The whole bucket is allowed for a template that deletes nothing.
	require.NoError(t, cmd.Flags().Set("prefix", ""))
	_, err = resolveLifecycleTemplate(cmd, "intelligent-tiering", "")
	require.NoError(t, err)
}

func TestListLifecycleTemplates_StatesDeletionAndTag(t *testing.T) {
	out := captureLifecycleStdout(t, func() { require.NoError(t, listLifecycleTemplates()) })
	assert.Contains(t, out, "DELETES all objects after 2555d", "the compliance expiration must be visible")
	assert.Contains(t, out, "cargoship-object=chunk")
	assert.Contains(t, out, "--prefix")
}

func captureLifecycleStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	return buf.String()
}

func TestLifecycleHelp_DocumentsPrefixAndExpiration(t *testing.T) {
	long := NewLifecycleCmd().Long
	for _, want := range []string{"--prefix", "cargoship-object=chunk", "DELETES", "2,555 days"} {
		assert.True(t, strings.Contains(long, want), "help should mention %q", want)
	}
}

package cmd

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func TestExistingFileFlags(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want manifest.ExistingFilePolicy
	}{
		{nil, manifest.ExistingRefuse},
		{[]string{"--overwrite"}, manifest.ExistingOverwrite},
		{[]string{"--skip-existing"}, manifest.ExistingSkip},
	} {
		var f existingFileFlags
		c := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
		f.add(c)
		c.SetArgs(tc.args)
		if err := c.Execute(); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if got := f.policy(); got != tc.want {
			t.Errorf("%v: policy = %v, want %v", tc.args, got, tc.want)
		}
	}

	var f existingFileFlags
	c := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	f.add(c)
	c.SetArgs([]string{"--overwrite", "--skip-existing"})
	c.SilenceErrors, c.SilenceUsage = true, true
	if err := c.Execute(); err == nil {
		t.Error("--overwrite and --skip-existing must be mutually exclusive")
	}
}

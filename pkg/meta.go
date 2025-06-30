package porter

import (
	"io"
	"log/slog"
	"os"
	"os/user"
	"path"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vjorlikowski/yaml"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
)

// CLIMeta is the command line meta information generated on a run
type CLIMeta struct {
	Username     string                 `yaml:"username"`
	Hostname     string                 `yaml:"hostname"`
	StartedAt    *time.Time             `yaml:"started_at"`
	CompletedAt  *time.Time             `yaml:"completed_at"`
	Arguments    []string               `yaml:"arguments"`
	ActiveFlags  map[string]interface{} `yaml:"active_flags"`
	DefaultFlags map[string]interface{} `yaml:"default_flags"`
	ViperConfig  map[string]interface{} `yaml:"viper_config"`
	Version      string                 `yaml:"version"`
	Wizard       *inventory.WizardForm  `yaml:"wizard"`
}

// MustComplete returns the completed file or panics if an error occurs
func (c *CLIMeta) MustComplete(od string) string {
	got, err := c.Complete(od)
	if err != nil {
		panic(err)
	}
	return got
}

// Complete is the final method for a CLI meta thing
func (c *CLIMeta) Complete(od string) (string, error) {
	if c == nil {
		return "", nil
	}
	n := time.Now()
	c.CompletedAt = &n

	var w io.WriteCloser
	var err error
	var mf string
	if od == "" {
		slog.Warn("No output directory specified. Using stdout for cli meta output")
		w = os.Stdout
	} else {
		mf = path.Join(od, "cargoship-invocation-meta.yaml")
		w, err = os.Create(mf) // nolint:gosec
		if err != nil {
			return "", err
		}
		slog.Debug("created CLI meta file", "meta-file", mf)
	}
	defer func() {
		err := w.Close()
		if err != nil {
			panic(err)
		}
	}()
	c.Print(w)
	return mf, nil
}

// Print writes the CLIMeta to an io.Writer
func (c *CLIMeta) Print(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		slog.Warn("could not marshal CLI metadata", "error", err)
		return
	}
	_, err = w.Write(b)
	if err != nil {
		slog.Warn("could not write CLI metadata", "error", err)
		return
	}
}

// CLIMetaOption is passed to create functional arguments for a new CLIMeta
type CLIMetaOption func(*CLIMeta)

const unknown = "UNKNOWN"

// NewCLIMeta returns a new CLIMeta option with functional options
func NewCLIMeta(opts ...CLIMetaOption) *CLIMeta {
	m := &CLIMeta{
		StartedAt: toPTR(time.Now()),
	}
	for _, opt := range opts {
		opt(m)
	}
	var err error
	m.Username, err = GetCurrentUser()
	if err != nil {
		slog.Warn("could not detect the current user", "error", err)
		m.Username = unknown
	}
	m.Hostname, err = os.Hostname()
	if err != nil {
		slog.Warn("Could not detect the current hostname", "error", err)
		m.Hostname = unknown
	}
	return m
}

// WithStart sets the start time for a CLIMeta
func WithStart(t *time.Time) CLIMetaOption {
	return func(m *CLIMeta) {
		m.StartedAt = t
	}
}

// WithMetaVersion sets the cargoship version in the meta
func WithMetaVersion(s string) CLIMetaOption {
	return func(m *CLIMeta) {
		m.Version = s
	}
}

// NewCLIMetaWithCobra returns a new CLIMeta option
func NewCLIMetaWithCobra(cmd *cobra.Command, args []string) *CLIMeta {
	start := time.Now()
	m := &CLIMeta{
		StartedAt:    &start,
		Arguments:    args,
		ActiveFlags:  map[string]interface{}{},
		DefaultFlags: map[string]interface{}{},
		Version:      cmd.Version,
	}
	// Yoink the CLI flags
	f := cmd.Flags()
	f.VisitAll(func(f *pflag.Flag) {
		if f.Changed {
			m.ActiveFlags[f.Name] = f.Value
		} else {
			m.DefaultFlags[f.Name] = f.Value
		}
	})

	var err error
	m.Username, err = GetCurrentUser()
	if err != nil {
		slog.Warn("Could not detect the current user", "error", err)
		m.Username = "Unknown"
	}
	m.Hostname, err = os.Hostname()
	if err != nil {
		slog.Warn("Could not detect the current hostname", "error", err)
		m.Hostname = "Unknown"
	}

	return m
}

// GetCurrentUser just returns the current user and an error if one occurred
func GetCurrentUser() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

func toPTR[V any](v V) *V {
	return &v
}

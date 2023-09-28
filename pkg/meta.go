package porter

import (
	"io"
	"os"
	"os/user"
	"path"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vjorlikowski/yaml"
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
	n := time.Now()
	c.CompletedAt = &n

	var w io.WriteCloser
	var err error
	var mf string
	if od == "" {
		log.Warn().Msg("No output directory specified. Using stdout for cli meta output")
		w = os.Stdout
	} else {
		mf = path.Join(od, "suitcasectl-invocation-meta.yaml")
		w, err = os.Create(mf) // nolint:gosec
		if err != nil {
			return "", err
		}
		log.Debug().Str("meta-file", mf).Msg("Created CLI meta file")
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
		log.Warn().Err(err).Msg("Could not marshal CLI meta data")
		return
	}
	_, err = w.Write(b)
	if err != nil {
		log.Warn().Err(err).Msg("Could not write CLI meta data")
		return
	}
}

// NewCLIMeta returns a new CLIMeta option
func NewCLIMeta(cmd *cobra.Command, args []string) *CLIMeta {
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
		log.Warn().Err(err).Msg("Could not detect the current user")
		m.Username = "Unknown"
	}
	m.Hostname, err = os.Hostname()
	if err != nil {
		log.Warn().Err(err).Msg("Could not detect the current hostname")
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

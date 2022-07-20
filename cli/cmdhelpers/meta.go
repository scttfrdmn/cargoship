package cmdhelpers

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

type CLIMeta struct {
	Username     string                 `yaml:"username"`
	Hostname     string                 `yaml:"hostname"`
	StartedAt    *time.Time             `yaml:"started_at"`
	CompletedAt  *time.Time             `yaml:"completed_at"`
	Arguments    []string               `yaml:"arguments"`
	ActiveFlags  map[string]interface{} `yaml:"active_flags"`
	DefaultFlags map[string]interface{} `yaml:"default_flags"`
	Version      string                 `yaml:"version"`
}

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
		w, err = os.Create(mf)
		if err != nil {
			return "", err
		}
		log.Info().Str("meta-file", mf).Msg("Created CLI meta file")
	}
	defer w.Close()
	c.Print(w)
	return mf, nil
}

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

func NewCLIMeta(args []string, cmd *cobra.Command) *CLIMeta {
	start := time.Now()
	m := &CLIMeta{
		StartedAt:    &start,
		Arguments:    args,
		ActiveFlags:  map[string]interface{}{},
		DefaultFlags: map[string]interface{}{},
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

func GetCurrentUser() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

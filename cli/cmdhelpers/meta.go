package cmdhelpers

import (
	"io"
	"os"
	"os/user"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v2"
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

func (c *CLIMeta) Complete() {
	n := time.Now()
	c.CompletedAt = &n
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

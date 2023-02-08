/*
Package cmd is the command line utility
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/pprof"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var (
	version = "dev"
	cfgFile string
	// Verbose uses lots more verbosity for output and logging and such
	Verbose bool
	trace   bool

	// Profiling data
	profile bool
	cpufile *os.File
)

// NewRootCmd represents the base command when called without any subcommands
func NewRootCmd(lo io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suitcasectl",
		Short:   "Used for creating encrypted blobs of files and directories for cold storage",
		Version: version,
		Long: `This tool generates a blob of encrypted files and directories that can be later
trasnfered to cheap archive storage. Along with the blob, an unencrypted
manifest file is generated. This manifest can be used to track down the blob at
a future point in time`,
		PersistentPreRun:  globalPersistentPreRun,
		PersistentPostRun: globalPersistentPostRun,
	}
	cmd.SetContext(context.WithValue(context.Background(), inventory.LogWriterKey, lo))
	// cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.suitcase.yaml)")
	cmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "Enable verbose output")
	cmd.PersistentFlags().BoolVarP(&trace, "trace", "t", false, "Enable trace messages in output")
	cmd.PersistentFlags().BoolVar(&profile, "profile", false, "Enable performance profiling. This will generate profile files in a temp directory")
	cmd.SetVersionTemplate("{{ .Version }}\n")

	// Create stuff
	createCmd := NewCreateCmd()

	createCmd.PersistentFlags().StringP("destination", "d", "", "Directory to write files in to. If not specified, we'll use an auto generated temp dir")
	if oerr := createCmd.MarkPersistentFlagDirname("destination"); oerr != nil {
		panic(oerr)
	}
	createSuitcaseCmd := NewCreateSuitcaseCmd()
	createCmd.AddCommand(createSuitcaseCmd)
	cmd.AddCommand(createCmd)

	cmd.AddCommand(NewCompletionCmd())
	cmd.AddCommand(NewSchemaCmd())
	cmd.SetOut(lo)

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// rootCmd = NewRootCmd()
	cobra.CheckErr(NewRootCmd(nil).Execute())
	// cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".cli" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".suitcasectl")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

/*
type runStats struct {
	Start   time.Time
	End     time.Time
	Runtime time.Duration
}
*/

func checkErr(err error, msg string) {
	if msg == "" {
		msg = "Fatal Error"
	}
	if err != nil {
		log.Fatal().Err(err).Msg(msg)
	}
}

func setupLogging(lo io.Writer) {
	if lo == nil {
		lo = os.Stderr
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if Verbose {
		log.Info().Msg("Verbose output enabled")
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	// If we have an outDir, also write the logs to a file
	multi := io.MultiWriter(zerolog.ConsoleWriter{Out: lo})
	if trace {
		log.Logger = zerolog.New(multi).With().Timestamp().Caller().Logger()
	} else {
		log.Logger = zerolog.New(multi).With().Timestamp().Logger()
	}
}

func setupMultiLoggingWithCmd(cmd *cobra.Command) error {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if Verbose {
		log.Info().Msg("Verbose output enabled")
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	// If we have an outDir, also write the logs to a file
	var multi io.Writer
	o, err := getDestination(cmd)
	if err != nil {
		return err
	}
	if o == "" {
		log.Warn().Msg("No output directory specified")
		return errors.New("no output directory specified")
	}
	/*
		logF, err = os.Create(cmd.Context().Value(logFileKey).(*os.File).Name()) // nolint:gosec
		if err != nil {
			return err
		}
	*/
	multi = io.MultiWriter(zerolog.ConsoleWriter{Out: cmd.OutOrStderr()}, cmd.Context().Value(inventory.LogFileKey).(*os.File))
	if trace {
		log.Logger = zerolog.New(multi).With().Timestamp().Caller().Logger()
	} else {
		log.Logger = zerolog.New(multi).With().Timestamp().Logger()
	}
	return nil
}

func globalPersistentPreRun(cmd *cobra.Command, args []string) {
	lo, ok := cmd.Context().Value(inventory.LogWriterKey).(io.Writer)
	if ok {
		setupLogging(lo)
	}
	// log.Fatal().Msgf("Profile is set to %+v", profile)
	if profile {
		log.Info().Msg("Enabling cpu profiling")
		var err error
		cpufile, err = os.CreateTemp("", "cpuprofile")
		if err != nil {
			panic(err)
		}
		err = pprof.StartCPUProfile(cpufile)
		if err != nil {
			panic(err)
		}
	}
}

func globalPersistentPostRun(cmd *cobra.Command, args []string) { // nolint:unparam
	if profile {
		pprof.StopCPUProfile()
		err := cpufile.Close()
		if err != nil {
			log.Warn().Err(err).Str("cpu-profile", cpufile.Name()).Msg("error closing cpu profiler")
		}
		log.Info().Str("cpu-profile", cpufile.Name()).Msg("CPU Profile Created")
	}

	// Empty out the outDir so multiple runs can happen
	// outDir = ""
	// cliMeta = nil
	// outDir = ""
	// logFile = ""
	// logF = nil
	// hashes = nil
	// userOverrides = nil
}

/*
Package cmd is the command line utility
*/
package cmd

import (
	"fmt"
	"io"
	"os"
	"path"
	"runtime/pprof"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/cmd/suitcasectl/cmdhelpers"
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

	cliMeta       *cmdhelpers.CLIMeta
	outDir        string
	logFile       string
	logF          *os.File
	hashes        []inventory.HashSet
	rootCmd       *cobra.Command
	userOverrides *viper.Viper
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
		// Uncomment the following line if your bare application
		// has an action associated with it:
		// Run: func(cmd *cobra.Command, args []string) { },
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			setupLogging(lo)
		},
		PersistentPostRun: globalPersistentPostRun,
	}
	// cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.suitcase.yaml)")
	cmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "Enable verbose output")
	cmd.PersistentFlags().BoolVarP(&trace, "trace", "t", false, "Enable trace messages in output")
	cmd.PersistentFlags().BoolVar(&profile, "profile", false, "Enable performance profiling. This will generate profile files in a temp directory")
	cmd.SetVersionTemplate("{{ .Version }}\n")
	cmd.AddCommand(createCmd)

	cmd.AddCommand(completionCmd)

	cmd.AddCommand(schemaCmd)
	cmd.SetOut(lo)

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// rootCmd = NewRootCmd()
	rootCmd = NewRootCmd(nil)
	cobra.CheckErr(rootCmd.Execute())
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

	// log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
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

	// log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if Verbose {
		log.Info().Msg("Verbose output enabled")
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	// If we have an outDir, also write the logs to a file
	var multi io.Writer
	// o := mustGetCmd[string](cmd, "destination")
	o, err := getDestination(cmd)
	if err != nil {
		return err
	}
	if o == "" {
		log.Fatal().Msg("No output directory specified")
	}
	logFile = path.Join(o, "suitcasectl.log")
	logF, err = os.Create(logFile) // nolint:gosec
	if err != nil {
		return err
	}
	multi = io.MultiWriter(zerolog.ConsoleWriter{Out: cmd.OutOrStderr()}, logF)
	if trace {
		log.Logger = zerolog.New(multi).With().Timestamp().Caller().Logger()
	} else {
		log.Logger = zerolog.New(multi).With().Timestamp().Logger()
	}
	return nil
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
}

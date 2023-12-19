/*
Package cmd is the command line utility
*/
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"runtime/pprof"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

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
		Short:   "Used blobs of files and directories for cold storage",
		Version: version,
		Long: paragraph(fmt.Sprintf(`The %s 🧳 tool generates bundles of files and directories that can be transferred off to remote storage. Files will be packaged by the maximum size of a suitcase.`,
			makeGradientText(lipgloss.NewStyle(), "suitcasectl"),
		)),
		PersistentPreRun:  globalPersistentPreRun,
		PersistentPostRun: globalPersistentPostRun,
		SilenceUsage:      true, // Usage too heavy to print out every time this thing fails
		SilenceErrors:     true, // We have a wrapper using our logger to do this
	}
	// fmt.Fprintf(os.Stderr, "LOGTHING: %+v\n", reflect.TypeOf(lo))
	// cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.suitcase.yaml)")
	cmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "Enable verbose output")
	cmd.PersistentFlags().BoolVarP(&trace, "trace", "t", false, "Enable trace messages in output")
	cmd.PersistentFlags().BoolVar(&profile, "profile", false, "Enable performance profiling. This will generate profile files in a temp directory")
	cmd.PersistentFlags().String("memory-limit", "", "Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.")
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
	rcloneCmd := NewRcloneCmd()
	cmd.AddCommand(rcloneCmd)

	cmd.AddCommand(NewRetierCmd())

	cmd.AddCommand(NewFindCmd())
	cmd.AddCommand(NewAnalyzeCmd())

	// cmd.AddCommand(NewCompletionCmd())
	cmd.AddCommand(NewSchemaCmd())
	cmd.AddCommand(NewMDDocsCmd())
	cmd.AddCommand(newManCmd())

	// cmd.SetContext(context.WithValue(context.Background(), inventory.LogWriterKey, lo))
	cmd.SetOut(lo)

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
/*
func Execute() {
	cobra.CheckErr(NewRootCmd(nil).Execute())
}
*/

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
		panic("must set lo")
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if Verbose {
		log.Info().Msg("Verbose output enabled")
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	// If we have an outDir, also write the logs to a file
	multi := io.MultiWriter(zerolog.ConsoleWriter{Out: lo})
	// multi := zerolog.MultiLevelWriter(lo, zerolog.ConsoleWriter{Out: os.Stderr})
	if trace {
		log.Logger = zerolog.New(multi).With().Timestamp().Caller().Logger()
	} else {
		log.Logger = zerolog.New(multi).With().Timestamp().Logger()
	}
}

func setupSingleLoggingWithCmd(cmd *cobra.Command) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if Verbose {
		log.Info().Msg("Verbose output enabled")
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	// log.Logger = log.Output(zerolog.ConsoleWriter{Out: cmd.OutOrStderr()}).With().Logger()
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: cmd.ErrOrStderr()}).With().Logger()
}

func setupMultiLoggingWithCmd(cmd *cobra.Command, args []string) error {
	// If we have an outDir, also write the logs to a file
	var multi io.Writer
	o, err := getDestination(cmd, args)
	if err != nil {
		return err
	}
	if o == "" {
		log.Warn().Msg("No output directory specified")
		return errors.New("no output directory specified")
	}
	ptr := mustPorterWithCmd(cmd, args)
	multi = io.MultiWriter(zerolog.ConsoleWriter{Out: cmd.OutOrStderr()}, ptr.LogFile)
	if trace {
		log.Logger = zerolog.New(multi).With().Timestamp().Caller().Logger()
	} else {
		log.Logger = zerolog.New(multi).With().Timestamp().Logger()
	}
	return nil
}

func globalPersistentPreRun(cmd *cobra.Command, args []string) {
	// Set up single logging first
	setupSingleLoggingWithCmd(cmd)

	// lo := cmd.OutOrStderr()
	// fmt.Fprintf(os.Stderr, "OUT IS: %+v\n", &lo)
	setupLogging(cmd.OutOrStderr())
	/*
		lo, ok := cmd.Context().Value(inventory.LogWriterKey).(io.Writer)
		if ok {
			setupLogging(lo)
		} else {
			log.Warn().Msg("could not set up log writer")
		}
	*/
	memLimit, err := cmd.Flags().GetString("memory-limit")
	checkErr(err, "Could not find memory limit")
	if memLimit != "" {
		memLimitB, merr := humanize.ParseBytes(memLimit)
		checkErr(merr, fmt.Sprintf("could not convert %v to bytes", memLimit))
		debug.SetMemoryLimit(int64(memLimitB))
		log.Info().Str("mem-limit", memLimit).Uint64("mem-limit-bytes", memLimitB).Msg("overriding memory handling with limit")
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

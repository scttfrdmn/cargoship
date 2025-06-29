/*
Package cmd is the command line utility
*/
package cmd

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"time"

	"github.com/charmbracelet/log"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	slogmulti "github.com/samber/slog-multi"
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
	logger  *slog.Logger
)

// NewRootCmd represents the base command when called without any subcommands
func NewRootCmd(lo io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cargoship",
		Short:   "Enterprise data archiving for AWS",
		Version: version,
		Long: paragraph(fmt.Sprintf(`The %s 🚢 tool ships your data to AWS with intelligence and efficiency. Built for enterprise-scale archiving with cost optimization and observability.`,
			makeGradientText(lipgloss.NewStyle(), "cargoship"),
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

	cmd.AddCommand(
		NewFindCmd(),
		NewTreeCmd(),
		NewEstimateCmd(),
		NewLifecycleCmd(),
		NewMetricsCmd(),
		NewConfigCmd(),
		NewBenchmarkCmd(),
	)
	cmd.AddCommand(NewWizardCmd())
	cmd.AddCommand(NewAnalyzeCmd())

	// cmd.AddCommand(NewCompletionCmd())
	cmd.AddCommand(NewSchemaCmd())
	cmd.AddCommand(NewMDDocsCmd())
	cmd.AddCommand(newManCmd(), newTravelAgentCmd())

	// cmd.SetContext(context.WithValue(context.Background(), inventory.LogWriterKey, lo))
	cmd.SetOut(lo)

	return cmd
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
		viper.SetConfigName(".cargoship")
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
		slog.Error(msg, "error", err)
		os.Exit(2)
	}
}

func newLoggerOpts() log.Options {
	logOpts := log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.Kitchen,
		Prefix:          "cargoship 🚢 ",
		Level:           log.InfoLevel,
		ReportCaller:    trace,
	}
	if Verbose {
		logOpts.Level = log.DebugLevel
	}

	return logOpts
}

func newJSONLoggerOpts() log.Options {
	logOpts := log.Options{
		ReportTimestamp: true,
		Prefix:          "cargoship",
		Level:           log.InfoLevel,
		ReportCaller:    trace,
		Formatter:       log.JSONFormatter,
	}
	if Verbose {
		logOpts.Level = log.DebugLevel
	}

	return logOpts
}

func setupLogging(w io.Writer) {
	if w == nil {
		panic("must set writer")
	}

	logger = slog.New(log.NewWithOptions(w, newLoggerOpts()))
	slog.SetDefault(logger)
}

func setupMultiLoggingWithCmd(cmd *cobra.Command) error {
	// If we have an outDir, also write the logs to a file
	o, err := getDestinationWithCobra(cmd)
	if err != nil {
		return err
	}
	if o == "" {
		return errors.New("no output directory specified")
	}
	ptr := mustPorterWithCmd(cmd)

	logger = slog.New(
		slogmulti.Fanout(
			log.NewWithOptions(cmd.OutOrStdout(), newLoggerOpts()),
			log.NewWithOptions(ptr.LogFile, newJSONLoggerOpts()),
		),
	)

	// Make sure the Porter object still has the right logger
	ptr.Logger = logger
	slog.SetDefault(logger)
	return nil
}

func globalPersistentPreRun(cmd *cobra.Command, _ []string) {
	// Set up single logging first
	// setupSingleLoggingWithCmd(cmd)

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
		debug.SetMemoryLimit(uint64ToInt64(memLimitB))
		slog.Info("overriding memory handling with limit", "mem-limit", memLimit, "mem-limit-bytes", memLimitB)
	}
	// log.Fatal().Msgf("Profile is set to %+v", profile)
	if profile {
		slog.Info("enabling cpu profiling")
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

func globalPersistentPostRun(_ *cobra.Command, _ []string) {
	if profile {
		pprof.StopCPUProfile()
		err := cpufile.Close()
		if err != nil {
			slog.Warn("error closing cpu profiler", "cpu-profile", cpufile.Name())
		}
		slog.Info("cpu profile created", "cpu-profile", cpufile.Name())
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

func toPTR[V any](v V) *V {
	return &v
}

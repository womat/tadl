package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"tadl/pkg/app"
	"tadl/pkg/app/config"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/womat/debug"
)

// Readme embeds the README.md file for the --help flag
//
//go:embed README.md
var Readme string

// buildDate and buildCommit are injected at build time via -ldflags
var (
	buildDate   = "dev"
	buildCommit = "none"
)

// @securityDefinitions.apikey	ApiKeyAuth
// @in							header
// @name						X-API-Key
func main() {
	// Parse command line flags.
	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.SetOutput(os.Stdout)

	about := flags.Bool("about", false, "Print app details and exit")
	help := flags.Bool("help", false, "Print a help message and exit")
	version := flags.Bool("version", false, "Print the app version and exit")
	debug := flags.Bool("debug", false, "Enable debug logging to stdout (overrides log settings from the config file)")
	configFile := flags.String("config", filepath.Join("/opt", app.MODULE, "etc", "config.yaml"), "Specify the path to the config file")

	if envCfg := os.Getenv("CONFIG_FILE"); envCfg != "" {
		*configFile = envCfg
	}

	if err := flags.Parse(os.Args[1:]); err != nil {
		fmt.Println("Error parsing flags:", err)
		flags.Usage()
		os.Exit(1)
	}

	switch {
	case *about:
		fmt.Println(About())
		os.Exit(0)
	case *version:
		fmt.Println(app.VERSION)
		os.Exit(0)
	case *help:
		fmt.Println(Readme)
		os.Exit(0)
	}

	// Run main application loop
	os.Exit(run(*configFile, *debug))
}

// run initializes configuration, logging, and the application loop.
// It supports hot-reloading of the configuration and handles graceful shutdown.
func run(configFile string, debug bool) int {

	var logger *xlog.LoggerWrapper
	defer func() {
		if logger != nil {
			logger.Close()
		}
	}()

	fmt.Printf("Starting %s %s\n", app.MODULE, app.VERSION)

	for {
		// Reload configuration on every restart
		config, err := loadConfig(configFile, debug)
		if err != nil {
			fmt.Printf("Failed to load config file %s: %s\n", configFile, err.Error())
			return 1
		}

		// Close previous logger if exists
		if logger != nil {
			logger.Close()
		}

		// Initialize logger
		if logger, err = xlog.Init(config.LogDestination, config.LogLevel); err != nil {
			fmt.Printf("Failed to initialize logger: %s\n", err.Error())
			return 1
		}

		slog.SetDefault(logger.Logger)
		slog.Info("Logging initialized/reloaded", "logLevel", config.LogLevel)

		// Create and run the application
		a, err := app.New(config, filepath.Join("/opt", app.MODULE)).Run()
		if err != nil {
			slog.Error("Critical error occurred, shutting down", "error", err)
			return 1
		}

		// Wait for restart or shutdown signals
		select {
		case <-a.Restart():
			slog.Info("Reloading configuration", "configFile", configFile)
			time.Sleep(time.Second) // prevent tight restart loops
		case <-a.Shutdown():
			slog.Info("Shutdown requested")
			return 0
		}
	}
}

func About() string {
	info := map[string]string{
		"Author":   "Wolfgang Mathe",
		"Binary":   filepath.Join("/opt", app.MODULE, "bin", app.MODULE),
		"Date":     buildDate,
		"Commit":   buildCommit,
		"Desc":     app.MODULE + " is demo app",
		"Help":     filepath.Join("/opt", app.MODULE, "bin", app.MODULE) + " --help",
		"Main":     filepath.Join("/opt/src", app.MODULE, "cmd", app.MODULE, "main.go"),
		"ProgLang": runtime.Version(),
		"Repo":     "https://github.com/womat/" + app.MODULE + ".git",
		"Version":  app.VERSION,
	}

	b, err := yaml.Marshal(info)
	if err != nil {
		return fmt.Sprintf("Failed to marshal About info: %v", err)
	}
	return string(b)
}

// loadConfig loads the configuration from a YAML file, applies overrides and validates the configuration values.
//
// If debug mode is enabled, log level is forced to "debug" and logs are written to stdout.
func loadConfig(configFile string, debug bool) (*app.Config, error) {

	config, err := app.LoadConfig(configFile)
	if err != nil {
		return nil, err
	}

	if debug {
		config.LogLevel = "debug"
		config.LogDestination = "stdout"
	}

	if err = config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

const defaultConfigFile = "/opt/womat/config/" + app.MODULE + ".yaml"

func main2() {
	exitCode := 1
	defer func() {
		os.Exit(exitCode)
	}()

	// cfg holds the application configuration
	cfg := config.NewConfig()

	cliApp := &cli.App{
		Name:    app.MODULE,
		Usage:   "UVR42 Datalogger for UVR42 Controller over DL-Bus",
		Version: app.VERSION,
		Description: "Read measurements of the UVR42 Controller and write values to mqtt" +
			"\n the UVR42 Controller is manufactured by Technische Alternative: https://www.ta.co.at" +
			"\n and the connection between UVR42 is implemented by DL-Bus (50Hz display clock).",
		UsageText: "tadl [--conf <file>] [--log error|debug|trace]" +
			"\n\nEXAMPLE:" +
			"\n\tstart the data logger and use the configuration file tadl.yaml" +
			"\n\t\ttadl --conf /opt/womat/tadl.yaml",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Destination: &cfg.Flag.ConfigFile, Value: defaultConfigFile, Usage: "load configuration from `FILE`"},
			&cli.StringFlag{Name: "log", Aliases: []string{"l"}, Destination: &cfg.Flag.LogLevel, Value: "standard", Usage: "`LEVEL` defines the log level (fatal|info|warning|error|debug|trace)"},
		},
		Action: func(ctx *cli.Context) error {
			if err := cfg.LoadConfig(); err != nil {
				return err
			}

			debug.SetDebug(cfg.Log.File, cfg.Log.Flag)
			defer func() {
				debug.InfoLog.Printf("closing debug file %s", cfg.Log.FileString)
				_ = cfg.Log.File.Close()
			}()

			a, err := app.New(cfg)
			defer func() {
				debug.InfoLog.Printf("closing app %s", app.Version())
				_ = a.Close()
			}()

			if err != nil {
				return err
			}

			debug.InfoLog.Printf("starting app %s", app.Version())
			if err = a.Run(); err != nil {
			}

			// capture exit signals to ensure resources are released on exit.
			quit := make(chan os.Signal)
			signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(quit)

			// wait for am os.Interrupt signal (CTRL C)
			sig := <-quit
			debug.InfoLog.Printf("Got %s signal. Aborting...", sig)

			return err
		},
	}

	// we expect to have more command line flags in the future - sort them
	sort.Sort(cli.FlagsByName(cliApp.Flags))
	sort.Sort(cli.CommandsByName(cliApp.Commands))

	err := cliApp.Run(os.Args)
	if err != nil {
		debug.FatalLog.Print(err)
		exitCode = 1
		return
	}

	exitCode = 0
	return
}

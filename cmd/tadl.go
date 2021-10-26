package main

import (
	"os"
	"os/signal"
	"sort"
	"syscall"

	"tadl/pkg/app"
	"tadl/pkg/app/config"

	"github.com/urfave/cli/v2"
	"github.com/womat/debug"
)

const defaultConfigFile = "/opt/womat/config/" + app.MODULE + ".yaml"

func main() {
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

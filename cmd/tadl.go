package main

// TODO: documentation

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"tadl/pkg/app"
	"tadl/pkg/app/config"

	"github.com/womat/debug"
)

const defaultConfigFile = "/opt/womat/config/" + app.MODULE + ".yaml"

func main() {
	exitCode := 1
	defer func() {
		os.Exit(exitCode)
	}()

	debug.SetDebug(os.Stderr, debug.Standard)
	cfg := config.NewConfig()

	flag.BoolVar(&cfg.Flag.Version, "version", false, "print version and exit")
	flag.StringVar(&cfg.Flag.Debug, "debug", "", "enable debug information (standard | trace | debug)")
	flag.StringVar(&cfg.Flag.ConfigFile, "config", defaultConfigFile, "config file")
	flag.Parse()

	if cfg.Flag.Version {
		fmt.Println(app.Version())
		exitCode = 0
		return
	}

	if err := cfg.LoadConfig(); err != nil {
		fmt.Println(err)
		exitCode = 1
		return
	}

	debug.SetDebug(cfg.Debug.File, cfg.Debug.Flag)
	defer func() {
		debug.InfoLog.Printf("closing debug file %s", cfg.Debug.FileString)
		_ = cfg.Debug.File.Close()
	}()

	debug.InfoLog.Printf("starting app %s", app.Version())
	a, err := app.New(cfg)
	defer func() {
		debug.InfoLog.Printf("closing app %s", app.Version())
		_ = a.Close()
	}()

	if err != nil {
		debug.FatalLog.Print(err)
		exitCode = 1
		return
	}

	if err := a.Run(); err != nil {
		debug.FatalLog.Print(err)
		exitCode = 1
		return
	}

	// capture exit signals to ensure resources are released on exit.
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	// wait for am os.Interrupt signal (CTRL C)
	sig := <-quit
	debug.InfoLog.Printf("Got %s signal. Aborting...", sig)

	exitCode = 1
	return
}

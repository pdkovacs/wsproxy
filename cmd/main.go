package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	wsproxy "wsproxy/internal"
	"wsproxy/internal/config"
	"wsproxy/internal/logging"
)

func main() {
	logger := logging.Get().With().Str("root", "main").Logger()
	ctx := logger.WithContext(context.Background())

	var serverWanted bool = true

	for _, value := range os.Args {
		if value == "-v" || value == "--version" {
			fmt.Print(config.GetBuildInfoString())
			serverWanted = false
		}
	}

	if serverWanted {
		var confErr error

		conf := config.GetConfig(os.Args)
		if confErr != nil {
			panic(confErr)
		}

		var stopServer func()
		exitc := make(chan struct{})

		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc,
			syscall.SIGHUP,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT)
		go func() {
			s := <-sigc
			fmt.Fprintf(os.Stderr, "Caught %v, stopping server...\n", s)
			stopServer()
			fmt.Fprintln(os.Stderr, "Server stopped")
			exitc <- struct{}{}
		}()

		app := wsproxy.NewServer(ctx, conf, func() wsproxy.ConnectionID {
			return wsproxy.CreateID(ctx)
		})
		errAppStart := app.SetupAndStart(func(port int, stop func()) {
			stopServer = stop
		})
		if errAppStart != nil {
			panic(errAppStart)
		}

		<-exitc
		fmt.Fprintln(os.Stderr, "Exiting...")
	}
}

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"wsproxy/internal/logging"
	"wsproxy/test/e2e/app"
	"wsproxy/test/e2e/app/config"

	"github.com/rs/zerolog"
)

const (
	WSPROXY_URL_ENVVAR_NAME = "WSPROXY_URL"
)

func main() {
	logger := logging.Get().Level(zerolog.GlobalLevel()).With().Str(logging.UnitLogger, "main").Logger()
	logger.Info().Msg("Test application instance starting...")
	ctx := logger.WithContext(context.Background())

	conf := config.ParseCommandLineArgs(os.Args)

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

	startErr := app.Start(
		ctx,
		conf,
		func() string {
			wsproxyURL := os.Getenv(WSPROXY_URL_ENVVAR_NAME)
			if len(wsproxyURL) == 0 {
				panic(fmt.Sprintf("Environment variable %s must be set", WSPROXY_URL_ENVVAR_NAME))
			}
			logger.Info().Str(WSPROXY_URL_ENVVAR_NAME, wsproxyURL)
			return wsproxyURL
		},
		func(port int, stop func()) {
			stopServer = stop
		},
	)
	if startErr != nil {
		logger.Error().Err(startErr).Msg("E2EApp failed to start")
		os.Exit(1)
	}
}

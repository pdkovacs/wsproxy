package app

import (
	"context"
	"wsproxy/test/e2e/app/config"
	"wsproxy/test/e2e/app/httpadapter"
)

func Start(ctx context.Context, conf config.Options, getWsproxyUrl func() string, ready func(port int, stop func())) error {
	server := httpadapter.NewServer(
		conf,
		getWsproxyUrl,
	)

	server.Start(conf, func(port int, stop func()) {
		ready(port, func() {
			stop()
		})
	})

	return nil
}

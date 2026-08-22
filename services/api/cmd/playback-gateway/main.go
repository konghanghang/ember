package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	logpkg "github.com/konghang/ember/backend/internal/logging"
	gatewaypkg "github.com/konghang/ember/backend/internal/playbackgateway"
)

func main() {
	if err := logpkg.Init(); err != nil {
		log.Printf("[PlaybackGateway] code=logging_init_failed errorType=%T", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := gatewaypkg.RunProcess(ctx); err != nil {
		log.Printf("[PlaybackGateway] code=process_failed errorType=%T", err)
		os.Exit(1)
	}
}

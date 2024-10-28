package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func newContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		force := false
		for sig := range c {
			if force {
				log.Println("force shutdown", "signal", sig.String())
				os.Exit(2)
			} else {
				log.Println("graceful shutdown", "signal", sig.String())
				cancel()
				force = true
			}
		}
	}()

	return ctx
}

func Execute(cmd *cobra.Command) {
	err := cmd.ExecuteContext(newContext())
	if err != nil {
		log.Panic(err)
	}
}

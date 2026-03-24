package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jlim/claude-p2p/node"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[claude-p2p] %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	n, err := node.New(ctx)
	if err != nil {
		return err
	}
	defer n.Close()

	if err := n.Run(ctx); err != nil && err != context.Canceled {
		return err
	}
	return nil
}

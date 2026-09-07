package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var input io.Reader
	info, err := os.Stdin.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, "getJS: cannot inspect stdin:", err)
		os.Exit(1)
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		input = os.Stdin
	}
	os.Exit(run(ctx, os.Args[1:], input, os.Stdout, os.Stderr))
}

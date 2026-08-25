package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err == nil {
		err = run(context.Background(), cfg)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

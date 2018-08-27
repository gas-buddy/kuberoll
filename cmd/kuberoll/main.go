package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jasisk/kuberoll/pkg/cmd"
	"k8s.io/kubernetes/pkg/util/interrupt"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	intr := interrupt.New(nil, cancel)
	command := cmd.New(ctx)
	runLoop := func() error {
		return command.Run()
	}
	if err := intr.Run(runLoop); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[0], err)
		os.Exit(1)
	}
}

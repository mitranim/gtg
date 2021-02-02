/*
Usage:

	go run ./examples/diamond.go
	go run ./examples/diamond.go <task>

Shaped like this:

	  A
	 / \
	v   v
	B   C
	 \ /
	  v
	  D
*/
package main

import (
	"context"
	"fmt"

	g "github.com/mitranim/gtg"
)

func main() {
	g.MustRun(context.Background(), A)
}

func A(task g.Task) error {
	// Allow B and C to run concurrently, wait on both.
	g.MustWait(task, g.Par(B, C))

	// Any logic here.
	fmt.Println(`[A] done`)
	return nil
}

func B(task g.Task) error {
	g.MustWait(task, D)

	// Any logic here.
	fmt.Println(`[B] done`)
	return nil
}

func C(task g.Task) error {
	g.MustWait(task, g.Opt(D))

	// Any logic here.
	fmt.Println(`[C] done`)
	return nil
}

func D(_ g.Task) error {
	return fmt.Errorf("arbitrary failure")
}

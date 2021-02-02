/*
Usage:

	go run ./examples/site.go
	go run ./examples/site.go <task>
*/
package main

import (
	"context"
	"fmt"

	g "github.com/mitranim/gtg"
)

func main() {
	g.MustRunCmd(build, templates, templatesW, styles)
}

func build(task g.Task) error {
	g.MustWait(task, g.Par(templates, styles))

	fmt.Println(`[build] done`)
	return nil
}

func templates(task g.Task) error {
	g.MustWait(task, styles)

	fmt.Println(`[templates] done`)
	return nil
}

func templatesW(task g.Task) error {
	g.MustWait(task, g.Opt(templates))

	fmt.Println(`[templatesW] done`)
	return nil
}

func styles(task g.Task) error {
	err := runCmd(task, "sass", "<some arg>")
	fmt.Println(`[styles] done with:`, err)
	return err
}

func clean(task g.Task) error {
	fmt.Println(`cleaning filesystem`)
	return nil
}

func runCmd(ctx context.Context, args ...string) error {
	// return fmt.Errorf("[cmd] command failed")
	_, err := fmt.Printf("running command: %q\n", args)
	return err
}

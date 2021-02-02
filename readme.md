## Overview

"Go task group" or "Go task graph". Utility for running tasks (each a superset of `context.Context`) in a single group with coordination and deduplication.

Good for CLI task orchestration, serving as a Go-based alternative to Make (see [#comparison](#comparison-with-make)) and a simple, flexible replacement for [Mage](https://github.com/magefile/mage) (see [#comparison](#comparison-with-mage)). May be useful for non-CLI applications.

API documentation: https://pkg.go.dev/github.com/mitranim/gtg.

## TOC

* [#Overview](#overview)
* [#Usage](#usage)
  * [#CLI Usage](#cli-usage)
* [#Comparisons](#comparisons)
  * [#Comparison with `"context"`](#comparison-with-context)
  * [#Comparison with Make](#comparison-with-make)
  * [#Comparison with Mage](#comparison-with-mage)
* [#Known Limitations and TODOs](#known-limitations-and-todos)
* [#License](#license)

## Usage

(For CLI, see [#below](#cli-usage).)

This example demonstrates a diamond-shaped task graph:

```
    A
   / \
  v   v
  B   C
   \ /
    v
    D
```

* Task A waits on tasks B and C.

* Both B and C wait on D.

* C allows D to fail and logs the error, while B requires D to succeed.

* Each function in the group is called _exactly_ once.

```golang
import g "github.com/mitranim/gtg"

func main() {
  g.MustRun(context.Background(), A)
}

func A(task g.Task) error {
  // Allow B and C to run concurrently, wait on both.
  g.MustWait(task, g.Par(B, C))

  // Any logic here.
  return nil
}

func B(task g.Task) error {
  return g.Wait(task, D)
}

func C(task g.Task) error {
  // Choose one line. Either of them will work.
  g.Log(g.Wait(task, D))
  g.MustWait(task, g.Opt(D))

  // Any logic here.
  return nil
}

func D(_ g.Task) error {
  return fmt.Errorf("arbitrary failure")
}
```

### CLI Usage

Reusing the `A B C D` task definitions from the example above:

```golang
import g "github.com/mitranim/gtg"

func main() {
  g.MustRunCmd(A, B, C, D)
}
```

Then from the command line:

```sh
# Print available tasks.
go run .

# Run a specific task.
go run . a
```

## Comparisons

### Comparison with `"context"`

* Gtg is an extension of `context`, adding support for running tasks as a graph, with mutual coordination and deduplication.

* Gtg is useful for orchestrating tasks from CLI.

### Comparison with Make

* Make is only for CLI. Gtg is a general library with CLI convenience features.

* Make runs shell scripts, Gtg runs Go. Both are good for different things.

* Go scripts are more portable than shell scripts. (My camel-breaking straw was Bash incompatibilities between Ubuntu and MacOS.)

### Comparison with Mage

* Mage is only for CLI. Gtg is a general library with CLI convenience features.

* Gtg has no separate installation. Just use `go run .` to auto-install, like other dependencies.

* Gtg has no special build system. No accidental conflicts in imports and declarations. No accidental log suppression or log spam. No need for special system variables. No multi-GiB hidden cache folder stuck forever on your system.

* Gtg has no implicit control flow. Just handle your errors. `Must` is conventional, explicit, and optional.

* Gtg is compatible with external watchers such as [Gow](https://github.com/mitranim/gow).

* Gtg is much smaller and simpler. It adds only the absolute minimum of new concepts: one additional method for `context.Context`. Everything else is utility functions defined in terms of that.

* Gtg is new and immature.

## Known Limitations and TODOs

* CLI usage needs better error logging. Currently we just panic, rendering an error message but also the call stack.

* Should provide an option or function to log task start/end/timing.

* Task identity is determined via function pointers, using unsafe hacks. May be unreliable, needs more testing.

* `Choose` and `RunCmd` allow to run only one task. We should provide shortcuts for selecting N tasks, which can be explicitly run concurrently via `Par` or serially via `Ser`.

* `Ser` should produce an error when the requested tasks happen to run in a different order due to some other tasks. Currently this is unchecked.

## License

https://unlicense.org

## Misc

I'm receptive to suggestions. If this library _almost_ satisfies you but needs changes, open an issue or chat me up. Contacts: https://mitranim.com/#contacts
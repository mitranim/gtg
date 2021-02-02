/*
"Go task group" or "Go task graph". Utility for running tasks (functions with a
context) as a single group with mutual coordination and deduplication.

Good for CLI task orchestration, serving as a Go-based alternative to Make and a
simple, flexible replacement for Mage (https://github.com/magefile/mage). May
be useful for non-CLI applications.

For examples and comparison with other tools, see the readme:
https://github.com/mitranim/gtg.
*/
package gtg

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"unsafe"
)

/*
Describes a task, which is a superset of `context.Context` but belongs to a task
group or task graph. Allows to retrieve OTHER tasks from the same group.

The method `Task` retrieves an existing task, or creates a new task, starting it
on another goroutine. Tasks are keyed by the identity of the task function. See
`TaskFunc` for the explanation of identity.
*/
type Task interface {
	context.Context
	Task(TaskFunc) Task
}

/*
Creates a new task group/graph. Runs `fun` as the first task in the group, on
another goroutine, and returns the same task.

Honoring context cancellation is up to the task function.
*/
func Start(ctx context.Context, fun TaskFunc) Task {
	return (&taskGroup{ctx: ctx}).task(fun)
}

// Shortcut for `Must(Run())`.
func MustRun(ctx context.Context, fun TaskFunc) {
	Must(Run(ctx, fun))
}

/*
Creates a new task group/graph. Runs `fun` as the first task in the group,
blocks until it finishes, and returns its error.

When the "main" task finishes, the context provided to all tasks in this group
is canceled.
*/
func Run(ctx context.Context, fun TaskFunc) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	return waitFor(Start(ctx, fun))
}

// Shortcut for `Must(Wait())`.
func MustWait(task Task, fun TaskFunc) {
	Must(Wait(task, fun))
}

/*
Finds or starts the other task in the current group (via `task.Task(fun)`) and
waits for it on the current goroutine, returning its error.
*/
func Wait(task Task, fun TaskFunc) error {
	return waitFor(task.Task(fun))
}

/*
Short for "optional". Wraps a task function, making its success optional. The
task will always run, but its error will simply be logged.

For a single dependency, this is no better than:

	Log(Wait(task, fun))

The main use is for composition:

	MustWait(task, Par(A, Opt(B), C))

This is a convenience feature for CLI scripts. Apps usually do their own
logging, and would write their own version of this function.
*/
func Opt(fun TaskFunc) TaskFunc {
	/**
	TODO: figure out how to give it a name other than `func1`. Tried bound methods
	and it didn't seem to help.
	*/
	return func(task Task) error {
		Log(Wait(task, fun))
		return nil
	}
}

/*
Short for "serial". Creates a task function that will wait on the given tasks
in a sequence.

As always, any task in the current group is run only once. A task that finished
earlier will not be called again. The actual order of task execution may not
match the order in `Ser`.

Currently in Gtg, parallel takes priority over serial; making sure that no other
task is trying to run everything in parallel is on the user.
*/
func Ser(funs ...TaskFunc) TaskFunc {
	// TODO: figure out how to give it a name other than `func1`.
	return func(task Task) error {
		for _, fun := range funs {
			err := Wait(task, fun)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

/*
Short for "parallel" (although "concurrent" would be more precise). Creates a
task function that will request all given tasks to be run concurrently.

As always, any task in the current group is run only once. A task that finished
earlier will not be called again.
*/
func Par(funs ...TaskFunc) TaskFunc {
	// TODO: figure out how to give it a name other than `func1`.
	return func(task Task) error {
		if len(funs) == 0 {
			return nil
		}

		if len(funs) == 1 {
			return Wait(task, funs[0])
		}

		wg := makeWaitGroup(len(funs))
		for _, fun := range funs {
			wg.add(task.Task(fun))
		}
		return wg.wait()
	}
}

/*
Convenience function for CLI. If the error is non-nil, logs it, otherwise
ignores it:

	Log(Wait(task, AnotherTask))
*/
func Log(err error) {
	if err != nil {
		_, _ = fmt.Fprintf(logOutput, "[gtg] error: %+v\n", err)
	}
}

/*
Panics if the error is non-nil. Allows shorter, cleaner task code, while keeping
control flow explicit. Gtg automatically handles panics in tasks, annotating
them with task names.
*/
func Must(err error) {
	if err != nil {
		panic(err)
	}
}

// Shortcut for `Must(RunCmd())`.
func MustRunCmd(funs ...TaskFunc) {
	Must(RunCmd(funs...))
}

/*
Convenience function for CLI. Selects one task function via `Choose`, using the
command line arguments from `os.Args`. Runs this task and returns its error.

CLI scripts can use the `MustRunCmd` shortcut.
*/
func RunCmd(funs ...TaskFunc) error {
	fun, err := Choose(os.Args[1:], funs)
	if err != nil {
		return err
	}
	return Run(context.Background(), fun)
}

/*
Matches task names against function names (case-insensitive), selecting exactly
one task function. Validates that all task names are "known", there are no
duplicates among task names and functions, and that exactly one function can be
selected. The returned error, if any, will list the "known" tasks derived from
function names.

CLI scripts can use the `MustRunCmd` shortcut.
*/
func Choose(names []string, funs []TaskFunc) (TaskFunc, error) {
	known, err := dedup(funs)
	if err != nil {
		return nil, err
	}

	var chosen taskFuncs
	for _, name := range names {
		fun := known.byTaskName(name)
		if fun == nil {
			return nil, fmt.Errorf(`unknown task %q; known tasks (case-insensitive): %q`, name, known.shortNames())
		}

		err := chosen.add(fun)
		if err != nil {
			return nil, err
		}
	}

	if len(chosen) == 0 {
		return nil, fmt.Errorf(`no task specified, please choose one; known tasks (case-insensitive): %q`, known.shortNames())
	}
	if len(chosen) > 1 {
		return nil, fmt.Errorf(`too many tasks specified, please choose one (case-insensitive): %q`, chosen.shortNames())
	}

	return chosen[0], nil
}

/*
Task functions may be invoked by `Start`, `Run`, `Task.Task`, and so on. They
shouldn't be called manually, because the purpose of this package is to
deduplicate tasks in the same group/graph.

Task functions may be statically defined or closures. All references to the same
static function have the same identity, while closures created by the same
function have different identities. Identity is used for deduplication.
*/
type TaskFunc func(Task) error

func (self TaskFunc) equalTaskName(name string) bool {
	return strings.EqualFold(name, self.ShortName())
}

func (self TaskFunc) equalTask(other TaskFunc) bool {
	return self.equal(other) || self.equalTaskName(other.ShortName())
}

func (self TaskFunc) equal(other TaskFunc) bool {
	return self.id() == other.id()
}

/*
Returns the function's name without the package path:

	func A(task Task) error {}
	TaskFunc(A).ShortName() // "A"
*/
func (self TaskFunc) ShortName() string {
	return funcShortName(self.longName())
}

func (self TaskFunc) longName() string {
	return runtime.FuncForPC(reflect.ValueOf(self).Pointer()).Name()
}

/*
Function identity, used as a task key. Might be fatally flawed. Go really
doesn't want us to compare functions by pointer.
*/
func (self TaskFunc) id() uintptr {
	return *(*uintptr)(unsafe.Pointer(&self))
}

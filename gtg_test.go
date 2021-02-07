package gtg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTaskGroup(t *testing.T) {
	t.Run("creation and deduplication", func(t *testing.T) {
		var group taskGroup

		task0 := group.Task(TaskFuncNop0)
		task1 := group.Task(TaskFuncNop1)
		task2 := group.Task(TaskFuncNop2)

		neq(nil, task0)
		neq(nil, task1)
		neq(nil, task2)

		neq(task0, task1)
		neq(task0, task2)
		neq(task1, task2)

		task3 := group.Task(TaskFuncNop0)
		task5 := group.Task(TaskFuncNop2)
		task4 := group.Task(TaskFuncNop1)

		eq(task0, task3)
		eq(task1, task4)
		eq(task2, task5)
	})

	t.Run("task starts immediately runs once", func(t *testing.T) {
		var group taskGroup

		var runs int
		fun := func(Task) error {
			runs++
			return nil
		}

		t.Run("first and only run", func(t *testing.T) {
			task := group.Task(fun)
			waitDone(task)
			eq(nil, task.Err())
			eq(1, runs)
		})

		t.Run("no second run", func(t *testing.T) {
			task := group.Task(fun)
			waitDone(task)
			eq(nil, task.Err())
			eq(1, runs)
		})
	})
}

/*
This verifies several things:

	* The view of the task context from the "inside" is not the same as from
	  the "outside".

	* The task context as seen from the "inside" (in the task function) behaves
	  normally, can be waited on and canceled externally, etc. This was broken in
	  the initial implementation.

	* The task context as seen from the "outside" reflects the behavior of the
	  task function, rather than the original context.
*/
func TestTaskContext(t *testing.T) {
	t.Run("reflect external context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		task := Start(ctx, TaskFuncDoneErr)

		notDone(task)
		eq(nil, task.Err())

		cancel()
		waitDone(task)
		eq(true, errors.Is(task.Err(), context.Canceled))
	})

	t.Run("wait on external context, override error", func(t *testing.T) {
		sentinel := fmt.Errorf(`sentinel`)

		fun := func(ctx Task) error {
			waitDone(ctx)
			return sentinel
		}

		ctx, cancel := context.WithCancel(context.Background())
		task := Start(ctx, fun)

		notDone(task)
		eq(nil, task.Err())

		cancel()
		waitDone(task)
		eq(true, errors.Is(task.Err(), sentinel))
	})
}

func TestRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go cancel()
	err := Run(ctx, TaskFuncDoneErr)
	eq(true, errors.Is(err, context.Canceled))
}

func TestWait(t *testing.T) {
	t.Run("all empty", func(t *testing.T) {
		task := Start(context.Background(), TaskFuncNop0)
		eq(nil, Wait(task, TaskFuncNop1))
	})

	t.Run("with error", func(t *testing.T) {
		task := Start(context.Background(), TaskFuncNop0)
		neq(nil, Wait(task, TaskFuncImmediateErr))
	})
}

func TestOpt(t *testing.T) {
	var buf strings.Builder
	defer swapLogOutput(&buf)()

	t.Run("without opt", func(t *testing.T) {
		task := Start(context.Background(), TaskFuncImmediateErr)
		waitDone(task)
		neq(nil, task.Err())
		eq("", buf.String())
	})

	t.Run("with opt", func(t *testing.T) {
		task := Start(context.Background(), Opt(TaskFuncImmediateErr))
		waitDone(task)
		eq(nil, task.Err())
		eq(true, strings.Contains(buf.String(), "[gtg] error"))
	})
}

/*
TODO:

	* Expose concurrency. Ensure that if the given functions are invoked
	  concurrently rather than serially, the test detects that.

	* Ensure that each function is invoked exactly once, in the right order, and
	  no more.

	* Ensure that if one of the functions returns an error, the sequence returns
	  that error and does not invoke any further functions.
*/
func TestSer(t *testing.T) {
	t.Skip()
}

/*
TODO:

	* Expose serial execution. Ensure that if the given functions are invoked
	  serially rather than concurrently, the test detects that.

	* Ensure that each function is invoked exactly once.

	* Ensure that `Par` waits for all functions to finish successfully.

	* Ensure that the moment one of the functions returns an error, `Par` returns
	  that error without waiting for the other functions to finish.
*/
func TestPar(t *testing.T) {
	t.Skip()
}

func TaskFuncNop0(Task) error { return nil }

func TaskFuncNop1(Task) error { return nil }

func TaskFuncNop2(Task) error { return nil }

func TaskFuncDoneErr(ctx Task) error {
	waitDone(ctx)
	return ctx.Err()
}

func TaskFuncImmediateErr(Task) error {
	return fmt.Errorf(`immediate error`)
}

func eq(expected interface{}, actual interface{}) {
	if !reflect.DeepEqual(expected, actual) {
		panic(fmt.Errorf(`
failed equality check
expected: %#v
actual:   %#v
`, expected, actual))
	}
}

func neq(expected interface{}, actual interface{}) {
	if reflect.DeepEqual(expected, actual) {
		panic(fmt.Errorf(`
failed inequality check: %#v
`, actual))
	}
}

func waitDone(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		panic("timed out")
	}
}

func notDone(ctx context.Context) {
	select {
	case <-ctx.Done():
		panic("prematurely done")
	default:
	}
}

func swapLogOutput(out io.Writer) func() {
	prev := logOutput
	logOutput = out
	return func() { logOutput = prev }
}

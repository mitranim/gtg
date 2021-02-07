package gtg

import (
  "context"
  "fmt"
  "io"
  "os"
  "reflect"
  "strings"
  "sync"
)

var logOutput io.Writer = os.Stderr

// May be made public when adding a `ChooseMany` function.
type taskFuncs []TaskFunc

func (self *taskFuncs) add(val TaskFunc) error {
  if val == nil {
    return fmt.Errorf(`unexpected nil task function`)
  }
  name := val.ShortName()
  if name == "" {
    return fmt.Errorf(`unexpected unnamed task function %#v`, val)
  }
  if self.hasTaskName(name) {
    return fmt.Errorf(`unexpected task function with duplicate name %q`, name)
  }
  if self.hasTask(val) {
    return fmt.Errorf(`unexpected duplicate task function %q`, val.longName())
  }
  *self = append(*self, val)
  return nil
}

func (self taskFuncs) byTaskName(name string) TaskFunc {
  for _, value := range self {
    if value.equalTaskName(name) {
      return value
    }
  }
  return nil
}

func (self taskFuncs) hasTaskName(name string) bool {
  return self.byTaskName(name) != nil
}

func (self taskFuncs) hasTask(val TaskFunc) bool {
  for _, value := range self {
    if value.equalTask(val) {
      return true
    }
  }
  return false
}

func (self taskFuncs) shortNames() []string {
  var out []string
  for _, value := range self {
    out = append(out, value.ShortName())
  }
  return out
}

/*
Not exported because: (1) it's extremely trivial; (2) it could lead to gotchas
when confused with `Wait`.
*/
func waitFor(ctx context.Context) error {
  <-ctx.Done()
  return ctx.Err()
}

func funcShortName(name string) string {
  ind := strings.LastIndex(name, ".")
  if ind >= 0 {
    return name[ind+1:]
  }
  return name
}

func dedup(funs []TaskFunc) (taskFuncs, error) {
  var out taskFuncs
  for _, fun := range funs {
    err := out.add(fun)
    if err != nil {
      return nil, err
    }
  }
  return out, nil
}

/*
Task group implementation. Every task is created within a group, and embeds a
reference to it.

Because a task group has a context, we could easily make it implement `Task` by
embedding this context. This would allow us to eliminate the `TaskGroup`
interface. The reason we don't is because it's unclear what constitutes `Done()`
and `Err()` for a task group. Should it simply delegate to the context, or
should it wait for the completion of every task? Should it return the error of
the first task, or accumulate them all? Clearly just embedding the context would
not be enough. We already provide `Start()` and `Run()` whose idea of `Done()`
and `Err()` is tied to the "main" task, which should be enough.
*/
type taskGroup struct {
  ctx context.Context
  sync.Mutex
  tasks map[uintptr]*task
}

func (self *taskGroup) Task(fun TaskFunc) Task {
  self.Lock()
  defer self.Unlock()

  id := fun.id()
  existing := self.tasks[id]
  if existing != nil {
    return existing
  }

  if self.tasks == nil {
    self.tasks = map[uintptr]*task{}
  }

  created := newTask(self.ctx, self, fun)
  self.tasks[id] = created

  go created.run()
  return created
}

func newTask(ctx context.Context, group *taskGroup, fun TaskFunc) *task {
  return &task{
    ctx:       ctx,
    taskGroup: group,
    fun:       fun,
    done:      make(chan struct{}),
  }
}

// Allows embedding under a private field name. Shouldn't be used in other
// places to avoid needless reader confusion.
type ctx = context.Context

type task struct {
  ctx
  *taskGroup
  fun     TaskFunc
  done    chan struct{}
  errLock sync.Mutex
  err     error
}

// Override `context.Context.Err()`.
func (self *task) Err() error {
  self.errLock.Lock()
  defer self.errLock.Unlock()
  return self.err
}

// Override `context.Context.Done()`.
func (self *task) Done() <-chan struct{} {
  return self.done
}

// Must be called exactly once.
func (self *task) run() {
  defer self.finalize()

  // A view of the task from the "inside".
  err := self.fun(struct {
    ctx
    *taskGroup
  }{
    self.ctx,
    self.taskGroup,
  })

  self.errLock.Lock()
  defer self.errLock.Unlock()
  self.err = err
}

/*
Must be deferred:

  defer self.finalize()

Known issue: `fmt.Errorf` obscures stacktraces in errors generated by 3rd party
error packages.
*/
func (self *task) finalize() {
  defer close(self.done)

  if self.err != nil {
    self.err = fmt.Errorf(`task %q finished with error: %w`, self.fun.ShortName(), self.err)
    return
  }

  val := recover()

  err, _ := val.(error)
  if err != nil {
    self.err = fmt.Errorf(`task %q panicked with error: %w`, self.fun.ShortName(), err)
    return
  }

  if val != nil {
    self.err = fmt.Errorf(`task %q panicked with non-error value %#v`, self.fun.ShortName(), val)
  }
}

/*
Similar to "golang.org/x/sync/errgroup".Group, but should abort on the first
error while the tasks are still running, without relying on context
cancellation. This is useful to us because tasks are a graph, not a tree, and
don't "own" each other. It's possible and somewhat reasonable to have the
following:

  A -> Par(B, C)
  D -> Par(Opt(B), C)
  B -> error -> A aborts, but D is still waiting on C

The implementation is somewhat complex and inefficient. TODO improve.
*/
type waitGroup struct {
  tasks []Task
  cases []reflect.SelectCase
}

func makeWaitGroup(size int) waitGroup {
  return waitGroup{
    tasks: make([]Task, 0, size),
    cases: make([]reflect.SelectCase, 0, size),
  }
}

func (self *waitGroup) add(task Task) {
  self.tasks = append(self.tasks, task)
  self.cases = append(self.cases, reflect.SelectCase{
    Dir:  reflect.SelectRecv,
    Chan: reflect.ValueOf(task.Done()),
  })
}

func (self *waitGroup) wait() error {
  for len(self.cases) > 0 {
    index, _, _ := reflect.Select(self.cases)
    task := self.remove(index)
    err := task.Err()
    if err != nil {
      return err
    }
  }
  return nil
}

func (self *waitGroup) remove(index int) Task {
  task := self.tasks[index]

  tasks := self.tasks
  copy(tasks[index:], tasks[index+1:])
  self.tasks = tasks[:len(tasks)-1]

  cases := self.cases
  copy(cases[index:], cases[index+1:])
  self.cases = cases[:len(cases)-1]

  return task
}

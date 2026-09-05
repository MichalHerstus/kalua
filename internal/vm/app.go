package vm

import (
	"fmt"
	"time"

	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
)

// PendingKind identifies the blocking operation a binding requested just
// before the coroutine yielded. Phase 1 supports exactly one outstanding op
// (the calling coroutine). The phase-2 session actor generalises this into an
// inbox of arbitrary events (WS messages, timers, async completions).
type PendingKind int

const (
	PendingNone PendingKind = iota
	PendingSleep
	PendingFormShow
	PendingDBQuery
)

// DBOperation represents a database operation to execute asynchronously
type DBOperation struct {
	Query    string
	Params   []interface{}
	IsExec   bool
	IsQuery  bool
	IsInsert bool
	Handle   interface{} // *sql.DB or *sql.Tx
}

// PendingOp records what the run loop must satisfy before resuming the
// coroutine, and the value to hand back to it (k.sleep returns nil).
type PendingOp struct {
	Kind   PendingKind
	Delay  time.Duration
	Form   string
	Resume lua.LValue
	DBOp   *DBOperation
}

// App drives a single Lua coroutine (the script's main) to completion. Every
// LState access for an App happens on the goroutine calling Run, so no locking
// is required — the same rule the phase-2 session actor depends on.
type App struct {
	L      *lua.LState
	cancel func()

	quitting bool

	// pending is registered by a blocking binding right before it yields and
	// read once by the run loop after Resume reports ResumeYield.
	pending *PendingOp

	// sess is the session this app belongs to (for Form stack, etc.)
	sess common.SessionInterface

	// suspendedForm is set when main coroutine yields on form.show
	suspendedForm string
}

// NewApp binds the scheduler to an existing sandboxed state.
func NewApp(L *lua.LState) *App {
	return &App{L: L}
}

// SetSession sets the session for this app.
func (a *App) SetSession(sess common.SessionInterface) {
	a.sess = sess
}

// Session returns the session for this app.
func (a *App) Session() common.SessionInterface {
	return a.sess
}

// Quitting reports whether k.quit() has been requested.
func (a *App) Quitting() bool { return a.quitting }

// RequestQuit flags a clean teardown at the next scheduler tick.
func (a *App) RequestQuit() { a.quitting = true }

// Block registers the pending op the run loop must satisfy before resuming,
// then suspends the coroutine running in L. Bindings must return the result of
// Block from the LGFunction so the VM switches threads to the Resumer.
func (a *App) Block(L *lua.LState, p *PendingOp) int {
	a.pending = p
	return L.Yield(p.Resume)
}

// ScheduleSleep suspends the current coroutine until Delay elapses.
func (a *App) ScheduleSleep(L *lua.LState, Delay time.Duration) int {
	return a.Block(L, &PendingOp{Kind: PendingSleep, Delay: Delay, Resume: lua.LNil})
}

// Run executes fn (the app's main) inside a coroutine and pumps it until it
// finishes, yields to an unknown pending op, or k.quit() is requested.
// Returns nil on normal completion, ErrSuspended if suspended on form.show,
// or an error on failure.
func (a *App) Run(fn *lua.LFunction) error {
	co, cancel := a.L.NewThread()
	a.cancel = cancel
	defer func() {
		if a.cancel != nil {
			a.cancel()
		}
	}()

	next := fn
	for {
		st, err, _ := a.L.Resume(co, next, lua.LNil)
		next = nil // only the first Resume carries the entry function

		if a.quitting {
			return nil
		}
		switch st {
		case lua.ResumeError:
			return err
		case lua.ResumeOK:
			return nil
		case lua.ResumeYield:
			// fall through to satisfy the pending op
		default:
			return fmt.Errorf("internal: unexpected Resume state %v", st)
		}

		op := a.pending
		a.pending = nil
		if op == nil {
			// The coroutine yielded without a recognized pending op — typically
			// an async binding (RunAsync) that suspends main() while a worker
			// goroutine finishes. Return a suspend status so the session actor
			// loop takes over and resumes the coroutine on async completion.
			return ErrSuspended
		}
		switch op.Kind {
		case PendingFormShow:
			// Main coroutine suspended on form.show - return special status
			// so the session actor can continue processing inbox messages.
			// The session will call ResumeMain when the form is closed.
			a.suspendedForm = op.Form
			return ErrSuspended
		case PendingNone:
			return fmt.Errorf("internal: coroutine yielded without a pending op")
		}
	}
}

// ErrSuspended is returned by App.Run when the main coroutine suspends on form.show.
var ErrSuspended = fmt.Errorf("suspended on form show")

// SuspendedForm returns the name of the form the main coroutine is suspended on,
// or empty string if not suspended.
func (a *App) SuspendedForm() string {
	return a.suspendedForm
}

// ResumeMain resumes the main coroutine after a form.show suspension.
// Should be called by the session when the form is closed.
func (a *App) ResumeMain() error {
	if a.suspendedForm == "" {
		return nil // not suspended
	}
	a.suspendedForm = ""

	// The main coroutine is the one we created in Run (co).
	// We need to resume it. But we don't have direct access to co here.
	// The session will call L.Resume on the stored coroutine.
	// This method just clears the suspended state.
	return nil
}

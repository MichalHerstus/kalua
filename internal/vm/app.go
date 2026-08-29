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
	PendingNone     PendingKind = iota
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
	L       *lua.LState
	cancel  func()

	quitting bool

	// pending is registered by a blocking binding right before it yields and
	// read once by the run loop after Resume reports ResumeYield.
	pending *PendingOp

	// sess is the session this app belongs to (for Form stack, etc.)
	sess common.SessionInterface
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
		switch op.Kind {
		case PendingSleep:
			if op.Delay > 0 {
				time.Sleep(op.Delay)
			}
		case PendingFormShow:
			// Form show is handled by the session actor - we just wait for the
			// session to Resume us when the Form closes
			// The actual waiting happens in the session actor
		case PendingNone:
			return fmt.Errorf("internal: coroutine yielded without a pending op")
		}
	}
}

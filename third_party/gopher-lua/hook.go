package lua

import "fmt"

// Hooker is a debug hook installed on an LState. It is invoked by the
// interpreter at the boundaries described by the concrete hook type. The
// implementation is ported from the edolphin-ydf/gopher-lua fork, which adds
// the debug.hook() / SetHook debugging facility missing from upstream.
type Hooker interface {
	// call is invoked with the currently executing frame. The hook may push a
	// callback and call it; any returned values are dropped.
	call(L *LState, cf *callFrame)
	String() string
}

// LHook fires for every line executed (unless it matches the previous line),
// calling the callback with ("line", linenumber).
type LHook struct {
	callback *LFunction
	line     int
}

func newLHook(callback *LFunction, line int) *LHook {
	return &LHook{callback: callback, line: line}
}

func (lh *LHook) call(L *LState, cf *callFrame) {
	currentline := cf.Fn.Proto.DbgSourcePositions[cf.Pc-1]
	if currentline != 0 && currentline != L.prevline && cf.Fn != lh.callback {
		L.reg.Push(lh.callback)
		L.reg.Push(LString("line"))
		L.reg.Push(LNumber(currentline))
		L.callR(2, 0, -1)
		L.prevline = currentline
	}
}

func (lh *LHook) String() string { return fmt.Sprintf("hook: %p", lh) }

// CTHook fires for every `count` instructions, calling the callback with
// ("count").
type CTHook struct {
	callback     *LFunction
	count        int
	currentCount int
}

func newCTHook(callback *LFunction, count int) *CTHook {
	return &CTHook{callback: callback, count: count}
}

func (ct *CTHook) call(L *LState, cf *callFrame) {
	ct.currentCount++
	if ct.currentCount == ct.count {
		L.reg.Push(ct.callback)
		L.reg.Push(LString("count"))
		L.callR(1, 0, -1)
		ct.currentCount = 0
	}
}

func (ct *CTHook) String() string { return fmt.Sprintf("hook: %p", ct) }

// CHook fires on every function call, calling the callback with ("call") unless
// the callback is the function being called (avoids recursion into the hook).
type CHook struct {
	callback *LFunction
}

func newCHook(callback *LFunction) *CHook { return &CHook{callback: callback} }

func (ch *CHook) call(L *LState, cf *callFrame) {
	if ch.callback != cf.Fn {
		L.reg.Push(ch.callback)
		L.reg.Push(LString("call"))
		L.callR(1, 0, -1)
	}
}

func (ch *CHook) String() string { return fmt.Sprintf("hook: %p", ch) }

// RHook fires on every function return, calling the callback with ("return")
// unless the callback is the function returning.
type RHook struct {
	callback *LFunction
}

func newRHook(callback *LFunction) *RHook { return &RHook{callback: callback} }

func (rh *RHook) call(L *LState, cf *callFrame) {
	if rh.callback != cf.Fn {
		L.reg.Push(rh.callback)
		L.reg.Push(LString("return"))
		L.callR(1, 0, -1)
	}
}

func (rh *RHook) String() string { return fmt.Sprintf("hook: %p", rh) }

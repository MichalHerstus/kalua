// Package web provides the HTTP server and WebSocket bridge for KALUA run mode.
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/coder/websocket"
	"github.com/yuin/gopher-lua"

	"kalua/internal/common"
	"kalua/internal/session"
	"kalua/internal/bindings"
	"kalua/internal/vm"
)

// WSTransport implements common.Transport over WebSocket.
type WSTransport struct {
	conn   *websocket.Conn
	ctx    context.Context
	sendCh chan common.OutboxMsg
	recvCh chan common.InboxMsg
	done   chan struct{}
}

func NewWSTransport(ctx context.Context, conn *websocket.Conn) *WSTransport {
	return &WSTransport{
		conn:   conn,
		ctx:    ctx,
		sendCh: make(chan common.OutboxMsg, 64),
		recvCh: make(chan common.InboxMsg, 64),
		done:   make(chan struct{}),
	}
}

func (t *WSTransport) Send(msg common.OutboxMsg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancel()
	return t.conn.Write(ctx, websocket.MessageText, data)
}

func (t *WSTransport) Recv() <-chan common.InboxMsg {
	return t.recvCh
}

func (t *WSTransport) Close() error {
	close(t.done)
	return t.conn.CloseNow()
}

func (t *WSTransport) Run() {
	// Outbox pump
	go func() {
		for {
			select {
			case msg, ok := <-t.sendCh:
				if !ok {
					return
				}
				data, err := json.Marshal(msg)
				if err != nil {
					continue
				}
				ctx, cancel := context.WithTimeout(t.ctx, 5*time.Second)
				if err := t.conn.Write(ctx, websocket.MessageText, data); err != nil {
					cancel()
					return
				}
				cancel()
			case <-t.done:
				return
			}
		}
	}()

	// Inbox reader
	for {
		_, data, err := t.conn.Read(t.ctx)
		if err != nil {
			close(t.recvCh)
			return
		}
		var msg common.InboxMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		select {
		case t.recvCh <- msg:
		case <-t.done:
			return
		}
	}
}

// NewSessionWithWSTransport creates a session with a WebSocket transport.
// This loads the script and creates the session with a WebSocket transport.
func NewSessionWithWSTransport(id, scriptPath string, opts bindings.Options, transport common.Transport, logger session.Logger) (*session.Session, error) {
	L := vm.New()
	app := vm.NewApp(L)
	env := bindings.Setup(L, app, opts, nil, logger)
	
	// Load and compile the script
	chunkFn, err := vm.LoadFile(L, scriptPath)
	if err != nil {
		L.Close()
		return nil, err
	}

	// Execute chunk to define main()
	if err := L.CallByParam(lua.P{Fn: chunkFn, NRet: 0, Protect: true}); err != nil {
		L.Close()
		return nil, err
	}

	// Get main function
	mainFn := L.GetGlobal("main")
	if mainFn == lua.LNil {
		L.Close()
		return nil, fmt.Errorf("main function not found")
	}
	mainLFn, ok := mainFn.(*lua.LFunction)
	if !ok {
		L.Close()
		return nil, fmt.Errorf("main is not a function")
	}

	// Create session with transport
	sess, err := session.NewWithTransport(id, L, app, env, transport, logger)
	if err != nil {
		return nil, err
	}

	// Start main in a coroutine (similar to original New)
	go func() {
		err := app.Run(mainLFn)
		if err != nil && err != vm.ErrSuspended {
			logger.Errorf("main error: %v", err)
			sess.SendOutbox(common.OutboxMsg{
				Type: "error",
				Msg:  err.Error(),
			})
		}
	}()

	return sess, nil
}
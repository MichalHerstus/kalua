//go:build !wasm

// Package cli implements the relay subcommand.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coder/websocket"

	"kalua/internal/host"
)

func relayCmd(args []string) int {
	fs := flag.NewFlagSet("relay", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		hostFlag = fs.String("host", "127.0.0.1", "Host to bind to")
		port     = fs.Int("port", 9090, "Port to bind to")
		verbose  = fs.Bool("v", false, "Verbose logging")
	)

	if err := fs.Parse(args); err != nil {
		return int(host.ExitUsage)
	}

	logger := host.NewLogger(*verbose)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/relay", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"http://127.0.0.1:*", "http://localhost:*", "http://[::1]:*"},
		})
		if err != nil {
			logger.Errorf("websocket accept: %v", err)
			return
		}
		defer c.CloseNow()

		handleRelayConnection(ctx, c, logger)
	})

	addr := fmt.Sprintf("%s:%d", *hostFlag, *port)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	logger.Printf("KALUA relay listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Errorf("server error: %v", err)
		return int(host.ExitError)
	}
	return int(host.ExitOK)
}

// RelayMessage represents a message in the relay protocol.
type RelayMessage struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Params  map[string]interface{} `json:"params,omitempty"`
	Result  interface{}            `json:"result,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

func handleRelayConnection(ctx context.Context, c *websocket.Conn, logger *host.Logger) {
	// Read messages from WebSocket
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			if status := websocket.CloseStatus(err); status != -1 {
				logger.Printf("relay ws closed by client (code %d)", status)
			} else {
				logger.Errorf("relay ws read: %v", err)
			}
			return
		}

		var msg RelayMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			logger.Errorf("relay unmarshal: %v", err)
			continue
		}

		result, err := handleRelayMethod(msg.Params)
		response := RelayMessage{
			ID:     msg.ID,
			Type:   msg.Type + "_resp",
			Result: result,
		}
		if err != nil {
			response.Error = err.Error()
		}

		responseData, _ := json.Marshal(response)
		ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := c.Write(ctx2, websocket.MessageText, responseData); err != nil {
			cancel()
			logger.Errorf("relay write: %v", err)
			return
		}
		cancel()
	}
}

func handleRelayMethod(params map[string]interface{}) (interface{}, error) {
	// This would dispatch to the appropriate handler based on the method type
	// For M3, we'll implement the dispatch to existing bindings
	// For now, return a placeholder
	return map[string]interface{}{"status": "not implemented"}, nil
}
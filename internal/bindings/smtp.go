// Package bindings implements the §5.7 SMTP bindings: k.smtp_connect /
// k.smtp_send / k.smtp_disconnect. Delivers mail over net/smtp; every call
// runs through the async worker pattern (§2.2) so the session stays live.
package bindings

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yuin/gopher-lua"
)

// smtpHandle wraps a connected SMTP client plus its auth identity.
type smtpHandle struct {
	client *smtp.Client
	host   string
	auth   smtp.Auth
	mu     sync.Mutex
}

// smtpHandles stores open SMTP connections by ID.
var smtpHandles = make(map[string]*smtpHandle)
var smtpHandlesMu sync.Mutex

// registerSMTP installs k.smtp_connect / k.smtp_send / k.smtp_disconnect.
func registerSMTP(e *Env) {
	// k.smtp_connect{host,port,user,pw,tls} -> handle
	e.register("smtp_connect", "email", func(L *lua.LState) int {
		opts := L.CheckTable(1)
		host := opts.RawGetString("host").String()
		port := opts.RawGetString("port")
		portInt := 25
		if port != lua.LNil {
			if n, ok := v(port).(float64); ok {
				portInt = int(n)
			}
		}
		user := opts.RawGetString("user").String()
		pw := opts.RawGetString("pw").String()
		tlsOn := lua.LVAsBool(opts.RawGetString("tls"))

		return runBlocking(e, L, func() (interface{}, error) {
			addr := fmt.Sprintf("%s:%d", host, portInt)
			c, err := smtp.Dial(addr)
			if err != nil {
				return nil, fmt.Errorf("smtp error: dial %s: %v", addr, err)
			}
			ok := false
			defer func() {
				if !ok {
					c.Close()
				}
			}()

			if tlsOn {
				if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
					return nil, fmt.Errorf("smtp error: starttls: %v", err)
				}
			}

			h := &smtpHandle{client: c, host: host}
			if user != "" || pw != "" {
				h.auth = smtp.PlainAuth("", user, pw, host)
			}
			id := fmt.Sprintf("smtp_%p", h)
			smtpHandlesMu.Lock()
			smtpHandles[id] = h
			smtpHandlesMu.Unlock()
			ok = true
			return id, nil
		}, nil)
	})

	// k.smtp_send{from,to,subject,body,attachments} -> {sent=true}
	e.register("smtp_send", "email", func(L *lua.LState) int {
		id := L.CheckString(1)
		opts := L.OptTable(2, L.NewTable())

		subject := opts.RawGetString("subject").String()
		body := opts.RawGetString("body").String()
		from := opts.RawGetString("from").String()
		to := opts.RawGetString("to").String()

		var toList []string
		if to != "" {
			for _, part := range strings.Split(to, ",") {
				if p := strings.TrimSpace(part); p != "" {
					toList = append(toList, p)
				}
			}
		}
		if len(toList) == 0 {
			L.RaiseError("smtp_send: 'to' is required")
			return 0
		}

		// Optional cc table or string.
		var ccList []string
		if cc := opts.RawGetString("cc"); cc != lua.LNil {
			if ccTbl, ok := cc.(*lua.LTable); ok {
				ccTbl.ForEach(func(_, v lua.LValue) {
					if s := strings.TrimSpace(v.String()); s != "" {
						ccList = append(ccList, s)
					}
				})
			} else {
				for _, p := range strings.Split(cc.String(), ",") {
					if x := strings.TrimSpace(p); x != "" {
						ccList = append(ccList, x)
					}
				}
			}
		}

		// attachments: table of paths, or string entries {name=path}.
		var attachments []string
		if att := opts.RawGetString("attachments"); att != lua.LNil {
			if attTbl, ok := att.(*lua.LTable); ok {
				attTbl.ForEach(func(_, v lua.LValue) {
					if p := v.String(); p != "" {
						attachments = append(attachments, p)
					}
				})
			} else if p := att.String(); p != "" {
				attachments = append(attachments, p)
			}
		}

		return runBlocking(e, L, func() (interface{}, error) {
			h, ok := lookupSMTP(id)
			if !ok {
				return nil, fmt.Errorf("smtp error: handle not found: %s", id)
			}
			h.mu.Lock()
			defer h.mu.Unlock()

			c := h.client
			if h.auth != nil {
				if err := c.Auth(h.auth); err != nil {
					return nil, fmt.Errorf("smtp error: auth: %v", err)
				}
			}
			if err := c.Mail(from); err != nil {
				return nil, fmt.Errorf("smtp error: mail: %v", err)
			}
			for _, r := range toList {
				if err := c.Rcpt(r); err != nil {
					return nil, fmt.Errorf("smtp error: rcpt %s: %v", r, err)
				}
			}
			for _, r := range ccList {
				if err := c.Rcpt(r); err != nil {
					return nil, fmt.Errorf("smtp error: rcpt(cc) %s: %v", r, err)
				}
			}
			w, err := c.Data()
			if err != nil {
				return nil, fmt.Errorf("smtp error: data: %v", err)
			}

			// Build the MIME message.
			var msg strings.Builder
			msg.WriteString("From: " + from + "\r\n")
			msg.WriteString("To: " + strings.Join(toList, ", ") + "\r\n")
			if len(ccList) > 0 {
				msg.WriteString("Cc: " + strings.Join(ccList, ", ") + "\r\n")
			}
			msg.WriteString("Subject: " + encodeSubject(subject) + "\r\n")

			boundary := fmt.Sprintf("kalua-boundary-%d", len(subject)+len(body)+1)
			if len(attachments) > 0 {
				msg.WriteString("MIME-Version: 1.0\r\n")
				msg.WriteString("Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n")
				msg.WriteString("\r\n")
				msg.WriteString("--" + boundary + "\r\n")
				msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
				msg.WriteString("\r\n")
				msg.WriteString(body + "\r\n")
				for _, path := range attachments {
					name := filepath.Base(path)
					data, err := os.ReadFile(path)
					if err != nil {
						return nil, fmt.Errorf("smtp error: attachment %s: %v", path, err)
					}
					msg.WriteString("--" + boundary + "\r\n")
					msg.WriteString("Content-Type: application/octet-stream; name=\"" + name + "\"\r\n")
					msg.WriteString("Content-Disposition: attachment; filename=\"" + name + "\"\r\n")
					msg.WriteString("Content-Transfer-Encoding: base64\r\n")
					msg.WriteString("\r\n")
					msg.WriteString(base64Chunk(data) + "\r\n")
				}
				msg.WriteString("--" + boundary + "--\r\n")
			} else {
				msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
				msg.WriteString("\r\n")
				msg.WriteString(body + "\r\n")
			}

			if _, err := w.Write([]byte(msg.String())); err != nil {
				w.Close()
				return nil, fmt.Errorf("smtp error: write: %v", err)
			}
			if err := w.Close(); err != nil {
				return nil, fmt.Errorf("smtp error: close data: %v", err)
			}
			_ = c.Reset()
			return map[string]interface{}{"sent": true}, nil
		}, nil)
	})

	// k.smtp_disconnect([handle]) — close one (or all) SMTP connections.
	e.register("smtp_disconnect", "email", func(L *lua.LState) int {
		id := L.OptString(1, "")
		return runBlocking(e, L, func() (interface{}, error) {
			smtpHandlesMu.Lock()
			if id == "" {
				for k, h := range smtpHandles {
					h.mu.Lock()
					h.client.Close()
					h.mu.Unlock()
					delete(smtpHandles, k)
				}
				smtpHandlesMu.Unlock()
				return nil, nil
			}
			h, ok := smtpHandles[id]
			if ok {
				delete(smtpHandles, id)
			}
			smtpHandlesMu.Unlock()
			if !ok {
				return nil, fmt.Errorf("smtp error: handle not found: %s", id)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			return nil, h.client.Close()
		}, nil)
	})
}

// lookupSMTP returns an open SMTP handle by id.
func lookupSMTP(id string) (*smtpHandle, bool) {
	smtpHandlesMu.Lock()
	defer smtpHandlesMu.Unlock()
	h, ok := smtpHandles[id]
	return h, ok
}

// encodeSubject encodes a subject for RFC 2047 when it contains non-ASCII.
func encodeSubject(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ReplaceAll(s, "\r\n", " ")
	}
	return "=?UTF-8?B?" + base64NoWrap(s) + "?="
}

// base64Chunk renders base64 in 76-char lines for MIME bodies.
func base64Chunk(data []byte) string {
	const line = 76
	enc := base64.StdEncoding.EncodeToString(data)
	var sb strings.Builder
	for len(enc) > 0 {
		n := len(enc)
		if n > line {
			n = line
		}
		sb.WriteString(enc[:n])
		sb.WriteString("\r\n")
		enc = enc[n:]
	}
	return strings.TrimRight(sb.String(), "\r\n")
}

// base64NoWrap encodes a string as single-line base64.
func base64NoWrap(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
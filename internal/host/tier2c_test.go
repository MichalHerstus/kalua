package host

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// fakeSMTPServer is a minimal in-process SMTP server that accepts one or more
// connections, greets with 220, and stores the DATA payload.
type fakeSMTPServer struct {
	ln      net.Listener
	payload []byte
	last    string
}

func startFakeSMTP(t *testing.T) *fakeSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTPServer{ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				fmt.Fprintf(c, "220 fake ESMTP\r\n")
				var payload strings.Builder
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.TrimRight(line, "\r\n")
					upper := strings.ToUpper(cmd)
					switch {
					case upper == "HELO" || strings.HasPrefix(upper, "EHLO"):
						fmt.Fprintf(c, "250-fake\r\n250 OK\r\n")
					case strings.HasPrefix(upper, "AUTH"):
						fmt.Fprintf(c, "235 OK\r\n")
					case strings.HasPrefix(upper, "MAIL FROM"):
						fmt.Fprintf(c, "250 OK\r\n")
					case strings.HasPrefix(upper, "RCPT TO"):
						fmt.Fprintf(c, "250 OK\r\n")
					case strings.HasPrefix(upper, "DATA"):
						fmt.Fprintf(c, "354 go ahead\r\n")
						for {
							l, err := r.ReadString('\n')
							if err != nil {
								return
							}
							payload.WriteString(l)
							if l == ".\r\n" {
								break
							}
						}
						s.payload = []byte(payload.String())
						fmt.Fprintf(c, "250 queued\r\n")
					case upper == "RSET":
						fmt.Fprintf(c, "250 OK\r\n")
					case upper == "QUIT":
						fmt.Fprintf(c, "221 bye\r\n")
						return
					default:
						fmt.Fprintf(c, "250 OK\r\n")
					}
				}
			}(conn)
		}
	}()
	return s
}

func (s *fakeSMTPServer) addr() string { return s.ln.Addr().String() }

func TestRun_SMTPFlow(t *testing.T) {
	srv := startFakeSMTP(t)
	defer srv.ln.Close()
	host, portStr, _ := net.SplitHostPort(srv.addr())
	port, _ := strconv.Atoi(portStr)

	out, code := runTestScript(t, fmt.Sprintf(`function main()
  local h = k.smtp_connect({host=%q, port=%d, user="u", pw="p"})
  if type(h) ~= "string" then k.error("smtp_connect handle") end
  local res = k.smtp_send(h, {from="a@x.com", to="b@y.com", subject="Hi", body="hello body"})
  if res.sent ~= true then k.error("smtp_send sent") end
  k.smtp_disconnect(h)
  k.quit()
end
`, host, port))
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
	if len(srv.payload) == 0 {
		t.Fatal("SMTP server received no DATA payload")
	}
	if !strings.Contains(string(srv.payload), "Subject: Hi") {
		t.Errorf("subject missing: %q", srv.payload)
	}
	if !strings.Contains(string(srv.payload), "hello body") {
		t.Errorf("body missing: %q", srv.payload)
	}
}

// fakePOP3Server is a minimal POP3 server with one stored message.
type fakePOP3Server struct {
	ln     net.Listener
	msg    string
	passed bool
}

func startFakePOP3(t *testing.T) *fakePOP3Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakePOP3Server{ln: ln, msg: "From: a\r\nSubject: test\r\n\r\nhello\r\n"}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				fmt.Fprintf(c, "+OK fake POP3 ready\r\n")
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.TrimRight(line, "\r\n")
					upper := strings.ToUpper(cmd)
					switch {
					case strings.HasPrefix(upper, "USER"):
						fmt.Fprintf(c, "+OK\r\n")
					case strings.HasPrefix(upper, "PASS"):
						s.passed = true
						fmt.Fprintf(c, "+OK logged in\r\n")
					case upper == "STAT":
						fmt.Fprintf(c, "+OK 1 %d\r\n", len(s.msg))
					case upper == "LIST":
						fmt.Fprintf(c, "+OK 1 messages\r\n1 %d\r\n.\r\n", len(s.msg))
					case strings.HasPrefix(upper, "RETR"):
						fmt.Fprintf(c, "+OK message follows\r\n%s.\r\n", s.msg)
						s.msg = strings.ReplaceAll(s.msg, "\r\n.", "\r\n..")
					case strings.HasPrefix(upper, "DELE"):
						fmt.Fprintf(c, "+OK deleted\r\n")
					case upper == "NOOP":
						fmt.Fprintf(c, "+OK\r\n")
					case upper == "QUIT":
						fmt.Fprintf(c, "+OK bye\r\n")
						return
					default:
						fmt.Fprintf(c, "-ERR unknown\r\n")
					}
				}
			}(conn)
		}
	}()
	return s
}

// addr returns the listener address (host:port).
func (s *fakePOP3Server) addr() string { return s.ln.Addr().String() }

func TestRun_POP3Flow(t *testing.T) {
	srv := startFakePOP3(t)
	defer srv.ln.Close()
	host, portStr, _ := net.SplitHostPort(srv.addr())
	port, _ := strconv.Atoi(portStr)

	out, code := runTestScript(t, fmt.Sprintf(`function main()
  local h = k.pop3_connect({host=%q, port=%d, user="u", pw="p"})
  local st = k.pop3_stat(h)
  if st.count ~= 1 then k.error("stat count " .. tostring(st.count)) end
  local lst = k.pop3_list(h)
  if #lst ~= 1 or lst[1].id ~= 1 then k.error("list") end
  local msg = k.pop3_retr(h, 1)
  if not msg:find("Subject: test") then k.error("retr subject") end
  k.pop3_noop(h)
  k.pop3_dele(h, 1)
  k.pop3_quit(h)
  k.quit()
end
`, host, port))
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
	if !srv.passed {
		t.Error("POP3 server never got PASS")
	}
}

// fakeFTPServer is a minimal FTP server with passive-mode RETR/STOR/LIST and a
// virtual file table.
type fakeFTPServer struct {
	ln     net.Listener
	files  map[string][]byte
	received []byte
}

func startFakeFTP(t *testing.T) *fakeFTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeFTPServer{ln: ln, files: map[string][]byte{"/hi.txt": []byte("hello ftp\n")}}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				fmt.Fprintf(c, "220 fake FTP\r\n")
				var dataLn net.Listener
				closeData := func() {
					if dataLn != nil {
						dataLn.Close()
						dataLn = nil
					}
				}
				defer closeData()
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.ToUpper(strings.TrimRight(line, "\r\n"))
					switch {
					case strings.HasPrefix(cmd, "USER"):
						fmt.Fprintf(c, "331 ok\r\n")
					case strings.HasPrefix(cmd, "PASS"):
						fmt.Fprintf(c, "230 logged in\r\n")
					case cmd == "TYPE I":
						fmt.Fprintf(c, "200 ok\r\n")
					case cmd == "EPSV" || strings.HasPrefix(cmd, "EPSV "):
						dl, _ := net.Listen("tcp", "127.0.0.1:0")
						dataLn = dl
						_, pStr, _ := net.SplitHostPort(dl.Addr().String())
						p, _ := strconv.Atoi(pStr)
						fmt.Fprintf(c, "229 Entering Extended Passive Mode (|||%d|)\r\n", p)
					case strings.HasPrefix(cmd, "PASV"):
						fmt.Fprintf(c, "227 Entering Passive Mode (127,0,0,1,%d,%d)\r\n", 0, 0)
					case strings.HasPrefix(cmd, "SIZE"):
						name := strings.TrimSpace(line[4:])
						if b, ok := s.files[name]; ok {
							fmt.Fprintf(c, "213 %d\r\n", len(b))
						} else {
							fmt.Fprintf(c, "550 not found\r\n")
						}
					case strings.HasPrefix(cmd, "RETR"):
						name := strings.TrimSpace(line[4:])
						b, ok := s.files[name]
						if !ok {
							fmt.Fprintf(c, "550 not found\r\n")
							continue
						}
						fmt.Fprintf(c, "150 opening\r\n")
						conn2, err := dataLn.Accept()
						if err == nil {
							conn2.Write(b)
							conn2.Close()
						}
						closeData()
						fmt.Fprintf(c, "226 transfer complete\r\n")
					case strings.HasPrefix(cmd, "STOR"):
						name := strings.TrimSpace(line[4:])
						fmt.Fprintf(c, "150 opening\r\n")
						conn2, err := dataLn.Accept()
						if err == nil {
							b, _ := io.ReadAll(conn2)
							s.files[name] = b
							s.received = b
							conn2.Close()
						}
						closeData()
						fmt.Fprintf(c, "226 transfer complete\r\n")
					case cmd == "LIST" || strings.HasPrefix(cmd, "LIST "):
						fmt.Fprintf(c, "150 opening\r\n")
						conn2, err := dataLn.Accept()
						if err == nil {
							for name := range s.files {
								fmt.Fprintf(conn2, "-rw-r--r-- 1 user group %d Jan 1 00:00 %s\r\n", len(s.files[name]), name)
							}
							conn2.Close()
						}
						closeData()
						fmt.Fprintf(c, "226 ok\r\n")
					case strings.HasPrefix(cmd, "CWD"):
						fmt.Fprintf(c, "250 ok\r\n")
					case strings.HasPrefix(cmd, "MKD"):
						fmt.Fprintf(c, "257 created\r\n")
					case strings.HasPrefix(cmd, "DELE"):
						name := strings.TrimSpace(line[4:])
						delete(s.files, name)
						fmt.Fprintf(c, "250 deleted\r\n")
					case strings.HasPrefix(cmd, "RNFR"):
						fmt.Fprintf(c, "350 ready\r\n")
					case strings.HasPrefix(cmd, "RNTO"):
						fmt.Fprintf(c, "250 renamed\r\n")
					case cmd == "QUIT":
						fmt.Fprintf(c, "221 bye\r\n")
						return
					default:
						fmt.Fprintf(c, "200 ok\r\n")
					}
					if dataLn != nil && (strings.HasPrefix(cmd, "RETR") || strings.HasPrefix(cmd, "STOR") || cmd == "LIST" || strings.HasPrefix(cmd, "LIST ")) {
						// data connection was closed above
					}
				}
			}(conn)
		}
	}()
	return s
}

// addr returns the listener address (host:port).
func (s *fakeFTPServer) addr() string { return s.ln.Addr().String() }

func TestRun_FTPFlow(t *testing.T) {
	srv := startFakeFTP(t)
	defer srv.ln.Close()
	host, portStr, _ := net.SplitHostPort(srv.addr())
	port, _ := strconv.Atoi(portStr)

	dataDir := t.TempDir()
	out, code := runTestScript(t, fmt.Sprintf(`function main()
  local h = k.ftp_connect(%q, %d, "u", "p")
  if type(h) ~= "string" then k.error("ftp_connect handle") end
  k.ftp_set_cwd(h, "/")
  if k.ftp_file_exists(h, "/hi.txt") ~= true then k.error("ftp_file_exists") end
  if k.ftp_file_exists(h, "/nope.txt") ~= false then k.error("ftp_file_exists missing") end
  k.ftp_get_file(h, "/hi.txt", %q)
  if k.file_load(%q) ~= "hello ftp\n" then k.error("ftp_get_file content") end
  k.file_save(%q, "upload data")
  k.ftp_put_file(h, %q, "/up.txt")
  k.ftp_create_dir(h, "/newdir")
  k.ftp_delete(h, "/hi.txt")
  k.ftp_rename(h, "/up.txt", "/ren.txt")
  local names = k.ftp_list(h)
  if #names == 0 then k.error("ftp_list empty") end
  k.ftp_disconnect(h)
  k.quit()
end
`, host, port, dataDir+"/out.txt", dataDir+"/out.txt", dataDir+"/in.txt", dataDir+"/in.txt"), dataDir)
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
	if string(srv.received) == "" {
		t.Error("FTP STOR never received data")
	}
}

// TestRun_SoapFlow uses an httptest SOAP-ish endpoint that echoes the request.
func TestRun_SoapFlow(t *testing.T) {
	var gotBody string
	mux := http.NewServeMux()
	mux.HandleFunc("/svc", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(200)
		io.WriteString(w, "<?xml version=\"1.0\"?><soap:Envelope><soap:Body><EchoResult>42</EchoResult></soap:Body></soap:Envelope>")
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	out, code := runTestScript(t, fmt.Sprintf(`function main()
  local res = k.webservice_run({url=%q, action="http://tempuri.org/Echo"}, {a=1, b="x"})
  if res.status ~= 200 then k.error("soap status " .. res.status) end
  if not res.body:find("EchoResult") then k.error("soap body") end
  k.quit()
end
`, ts.URL+"/svc"))
	if code != ExitOK {
		t.Errorf("Run = %d, want ExitOK\noutput:\n%s", code, out)
	}
	if !strings.Contains(gotBody, "<Echo") || !strings.Contains(gotBody, "<a>1</a>") {
		t.Errorf("SOAP request body not as expected: %q", gotBody)
	}
}
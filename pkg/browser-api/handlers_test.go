package browser_api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// doAction calls executeBrowserAction with the given vars and body and
// returns the recorded response.
func doAction(t *testing.T, vars map[string]string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/action", strings.NewReader(body))
	req = mux.SetURLVars(req, vars)
	rec := httptest.NewRecorder()
	executeBrowserAction(rec, req)
	return rec
}

// useUpstream points the handler at upstreamUrl for the duration of the test.
func useUpstream(t *testing.T, upstreamUrl string) {
	t.Helper()
	orig := actionUrl
	actionUrl = func(string, string) string { return upstreamUrl }
	t.Cleanup(func() { actionUrl = orig })
}

var testVars = map[string]string{"namespace": "ns", "name": "browser"}

func TestExecuteBrowserActionValidation(t *testing.T) {
	// Validation must reject the request before anything is proxied.
	useUpstream(t, "http://127.0.0.1:0/unreachable")

	tests := []struct {
		name    string
		vars    map[string]string
		body    string
		wantMsg string
	}{
		{name: "missing name", vars: map[string]string{"namespace": "ns"}, body: `{"kind":"page-reload"}`,
			wantMsg: "namespace or name"},
		{name: "missing namespace", vars: map[string]string{"name": "browser"}, body: `{"kind":"page-reload"}`,
			wantMsg: "namespace or name"},
		{name: "invalid json", vars: testVars, body: `{`, wantMsg: "unexpected end"},
		{name: "unknown kind", vars: testVars, body: `{"kind":"rm-rf"}`, wantMsg: "'kind' must be one of"},
		{name: "empty kind", vars: testVars, body: `{}`, wantMsg: "'kind' must be one of"},
		{name: "navigate without url", vars: testVars, body: `{"kind":"page-navigate"}`,
			wantMsg: "'url' parameter is required"},
		{name: "url on non-navigate", vars: testVars, body: `{"kind":"page-reload","url":"https://example.com"}`,
			wantMsg: "'url' parameter is not allowed"},
		{name: "file url", vars: testVars, body: `{"kind":"page-navigate","url":"file:///etc/passwd"}`,
			wantMsg: "absolute http or https"},
		{name: "javascript url", vars: testVars, body: `{"kind":"page-navigate","url":"javascript:alert(1)"}`,
			wantMsg: "absolute http or https"},
		{name: "chrome url", vars: testVars, body: `{"kind":"page-navigate","url":"chrome://settings"}`,
			wantMsg: "absolute http or https"},
		{name: "relative url", vars: testVars, body: `{"kind":"page-navigate","url":"example.com"}`,
			wantMsg: "absolute http or https"},
		{name: "http url without host", vars: testVars, body: `{"kind":"page-navigate","url":"http:///path"}`,
			wantMsg: "absolute http or https"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doAction(t, tt.vars, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
				t.Errorf("Content-Type = %q, want text/plain", ct)
			}
			if !strings.Contains(rec.Body.String(), tt.wantMsg) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantMsg)
			}
		})
	}
}

func TestExecuteBrowserActionProxies(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "reload", body: `{"kind":"page-reload"}`},
		{name: "navigate http", body: `{"kind":"page-navigate","url":"http://example.com/a?b=1"}`},
		{name: "navigate https", body: `{"kind":"page-navigate","url":"https://example.com"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				w.WriteHeader(http.StatusAccepted)
			}))
			defer upstream.Close()
			useUpstream(t, upstream.URL)

			rec := doAction(t, testVars, tt.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
			}
			if gotBody != tt.body {
				t.Errorf("upstream body = %q, want %q", gotBody, tt.body)
			}
		})
	}
}

func TestExecuteBrowserActionUpstreamErrors(t *testing.T) {
	t.Run("upstream error status", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer upstream.Close()
		useUpstream(t, upstream.URL)

		rec := doAction(t, testVars, `{"kind":"page-reload"}`)
		if rec.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
		}
		if !strings.Contains(rec.Body.String(), "'500'") || !strings.Contains(rec.Body.String(), "boom") {
			t.Errorf("body = %q, want upstream status and body", rec.Body.String())
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		upstream := httptest.NewServer(http.NotFoundHandler())
		closedUrl := upstream.URL
		upstream.Close()
		useUpstream(t, closedUrl)

		rec := doAction(t, testVars, `{"kind":"page-reload"}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d (body %q)", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "not running") {
			t.Errorf("body = %q, want a friendly message", rec.Body.String())
		}
	})

	t.Run("timeout", func(t *testing.T) {
		release := make(chan struct{})
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-release
		}))
		defer upstream.Close()
		defer close(release)
		useUpstream(t, upstream.URL)

		origTimeout := browserServerClient.Timeout
		browserServerClient.Timeout = 50 * time.Millisecond
		t.Cleanup(func() { browserServerClient.Timeout = origTimeout })

		rec := doAction(t, testVars, `{"kind":"page-reload"}`)
		if rec.Code != http.StatusGatewayTimeout {
			t.Errorf("status = %d, want %d (body %q)", rec.Code, http.StatusGatewayTimeout, rec.Body.String())
		}
	})
}

func TestConnectToBrowserVNCMissingParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/vnc", nil)
	req = mux.SetURLVars(req, map[string]string{"namespace": "ns"})
	rec := httptest.NewRecorder()
	connectToBrowserVNC(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// dialState connects to watchBrowserState against upstream. The connection is closed when the test ends.
func dialState(t *testing.T, vars map[string]string, upstream http.HandlerFunc) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	upstreamServer := httptest.NewServer(upstream)
	t.Cleanup(upstreamServer.Close)
	orig := stateUrl
	stateUrl = func(string, string) string { return upstreamServer.URL }
	t.Cleanup(func() { stateUrl = orig })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		watchBrowserState(w, mux.SetURLVars(r, vars))
	}))
	t.Cleanup(server.Close)

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err == nil {
		t.Cleanup(func() { _ = conn.Close() })
	}
	return conn, resp, err
}

func writeStateLine(t *testing.T, w http.ResponseWriter, line string) {
	t.Helper()
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		t.Error(err)
	}
	w.(http.Flusher).Flush()
}

func readState(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return strings.TrimSpace(string(message))
}

func TestWatchBrowserState(t *testing.T) {
	t.Run("relays known fields as they arrive, then closes", func(t *testing.T) {
		next := make(chan struct{})
		conn, _, err := dialState(t, testVars, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("upstream method = %s, want GET", r.Method)
			}
			writeStateLine(t, w, `{"url":"https://example.com/a","extra":"dropped"}`)
			<-next
			writeStateLine(t, w, `{"url":"https://example.com/b"}`)
		})
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		if got, want := readState(t, conn), `{"url":"https://example.com/a"}`; got != want {
			t.Errorf("first message = %q, want %q", got, want)
		}
		close(next)
		if got, want := readState(t, conn), `{"url":"https://example.com/b"}`; got != want {
			t.Errorf("second message = %q, want %q", got, want)
		}
		if _, _, err := conn.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseGoingAway) {
			t.Errorf("after the stream ended: err = %v, want a going-away close", err)
		}
	})

	t.Run("stops reading upstream when the client disconnects", func(t *testing.T) {
		upstreamDone := make(chan struct{})
		conn, _, err := dialState(t, testVars, func(w http.ResponseWriter, r *http.Request) {
			writeStateLine(t, w, `{"url":"https://example.com/a"}`)
			<-r.Context().Done()
			close(upstreamDone)
		})
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		readState(t, conn)
		_ = conn.Close()
		select {
		case <-upstreamDone:
		case <-time.After(5 * time.Second):
			t.Error("upstream request was not cancelled")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		_, resp, err := dialState(t, map[string]string{"namespace": "ns"}, func(w http.ResponseWriter, r *http.Request) {
			t.Error("upstream must not be called")
		})
		if err == nil {
			t.Fatal("dial succeeded, want a failed handshake")
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("not a websocket request", func(t *testing.T) {
		req := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/state", nil), testVars)
		rec := httptest.NewRecorder()
		watchBrowserState(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("upstream error status", func(t *testing.T) {
		_, resp, err := dialState(t, testVars, func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
		if err == nil {
			t.Fatal("dial succeeded, want a failed handshake")
		}
		if resp.StatusCode != http.StatusBadGateway {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
		}
	})

	t.Run("invalid upstream json", func(t *testing.T) {
		conn, _, err := dialState(t, testVars, func(w http.ResponseWriter, r *http.Request) {
			writeStateLine(t, w, `<html>`)
		})
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		if _, _, err := conn.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseGoingAway) {
			t.Errorf("err = %v, want a going-away close", err)
		}
	})
}

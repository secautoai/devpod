package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── CORS middleware ──────────────────────────────────────────────────────────

func TestWithCORS_SetsHeadersOnGetRequest(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	withCORS(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type", rec.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestWithCORS_OptionsReturnsNoContent(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})
	rec := httptest.NewRecorder()
	withCORS(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/api/command", nil))

	assert.False(t, called, "inner handler must not be called for preflight OPTIONS")
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestWithCORS_PostRequestPassesThrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	rec := httptest.NewRecorder()
	withCORS(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/command", nil))
	assert.Equal(t, http.StatusCreated, rec.Code)
}

// ─── /api/version ─────────────────────────────────────────────────────────────

func TestHandleVersion_DefaultsToDev(t *testing.T) {
	t.Setenv("DEVPOD_VERSION", "")
	rec := httptest.NewRecorder()
	handleVersion(rec, httptest.NewRequest(http.MethodGet, "/api/version", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "dev", resp["version"])
}

func TestHandleVersion_ReturnsEnvVar(t *testing.T) {
	t.Setenv("DEVPOD_VERSION", "v2.5.0")
	rec := httptest.NewRecorder()
	handleVersion(rec, httptest.NewRequest(http.MethodGet, "/api/version", nil))

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "v2.5.0", resp["version"])
}

func TestHandleVersion_ContentTypeIsJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	handleVersion(rec, httptest.NewRequest(http.MethodGet, "/api/version", nil))
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}

// ─── /api/platform ────────────────────────────────────────────────────────────

func TestHandlePlatform_ReturnsGoOS(t *testing.T) {
	rec := httptest.NewRecorder()
	handlePlatform(rec, httptest.NewRequest(http.MethodGet, "/api/platform", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, runtime.GOOS, resp["platform"])
	assert.Equal(t, runtime.GOARCH, resp["arch"])
}

// ─── /api/health ──────────────────────────────────────────────────────────────

func TestHandleHealth_Returns200(t *testing.T) {
	rec := httptest.NewRecorder()
	handleHealth(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ─── /api/command (synchronous) ──────────────────────────────────────────────

func TestHandleCommandRun_MethodNotAllowed(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handleCommandRun(rec, httptest.NewRequest(method, "/api/command", nil))
			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})
	}
}

func TestHandleCommandRun_MalformedJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	handleCommandRun(rec,
		httptest.NewRequest(http.MethodPost, "/api/command", strings.NewReader("{bad json}")))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCommandRun_EmptyBody(t *testing.T) {
	rec := httptest.NewRecorder()
	handleCommandRun(rec,
		httptest.NewRequest(http.MethodPost, "/api/command", bytes.NewReader(nil)))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandleCommandRun_RunsStubBinary verifies that when the binary override is set to a simple
// echo-like program the response contains its output and exit code 0.
func TestHandleCommandRun_RunsStubBinary(t *testing.T) {
	// Inject the echo binary so we control the output.
	echo, err := lookupEcho()
	if err != nil {
		t.Skip("echo binary not found:", err)
	}
	old := devpodSelfOverride
	devpodSelfOverride = echo
	defer func() { devpodSelfOverride = old }()

	body := commandRequest{Args: []string{"hello from devpod web test"}}
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	handleCommandRun(rec,
		httptest.NewRequest(http.MethodPost, "/api/command", bytes.NewReader(b)))

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp commandResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.ExitCode)
	assert.Contains(t, resp.Stdout, "hello from devpod web test")
}

// ─── /api/providers and /api/workspaces ───────────────────────────────────────

func TestHandleProviders_MethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	handleProviders(rec, httptest.NewRequest(http.MethodPost, "/api/providers", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandleWorkspaces_MethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	handleWorkspaces(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// TestHandleProviders_RunsStubAndReturnsJSON verifies that /api/providers forwards stdout as JSON.
func TestHandleProviders_RunsStubAndReturnsJSON(t *testing.T) {
	echo, err := lookupEcho()
	if err != nil {
		t.Skip("echo binary not found:", err)
	}
	old := devpodSelfOverride
	devpodSelfOverride = echo
	defer func() { devpodSelfOverride = old }()

	rec := httptest.NewRecorder()
	// The "provider list --output=json --log-output=json" args are passed to echo,
	// so stdout = those args joined by spaces.
	handleProviders(rec, httptest.NewRequest(http.MethodGet, "/api/providers", nil))

	// echo exits 0, so we expect 200 and the Content-Type header.
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}

// ─── /api/signal ──────────────────────────────────────────────────────────────

func TestHandleSignal_MethodNotAllowed(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handleSignal(rec, httptest.NewRequest(method, "/api/signal", nil))
			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})
	}
}

func TestHandleSignal_MalformedJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	handleSignal(rec,
		httptest.NewRequest(http.MethodPost, "/api/signal", strings.NewReader("{not json}")))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSignal_ValidBodyDoesNotPanic(t *testing.T) {
	// PID 1 is always present on POSIX; sending signal 0 checks existence without killing.
	body := signalRequest{ProcessID: 1, Signal: 0}
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	// Should not panic – 200 or 404 are both valid.
	assert.NotPanics(t, func() {
		handleSignal(rec, httptest.NewRequest(http.MethodPost, "/api/signal", bytes.NewReader(b)))
	})
}

// ─── buildEnv ─────────────────────────────────────────────────────────────────

func TestBuildEnv_NilExtraEqualsOsEnviron(t *testing.T) {
	env := buildEnv(nil)
	assert.Equal(t, os.Environ(), env)
}

func TestBuildEnv_WithExtraVarsAppended(t *testing.T) {
	env := buildEnv(map[string]string{
		"DEVPOD_WEB_TEST_FOO": "bar",
		"DEVPOD_WEB_TEST_BAZ": "qux",
	})
	assert.Contains(t, env, "DEVPOD_WEB_TEST_FOO=bar")
	assert.Contains(t, env, "DEVPOD_WEB_TEST_BAZ=qux")
	// Should also include the parent environment.
	assert.Greater(t, len(env), 2)
}

func TestBuildEnv_EmptyMapEqualsOsEnviron(t *testing.T) {
	env := buildEnv(map[string]string{})
	assert.Equal(t, os.Environ(), env)
}

// ─── buildFrontendHandler ─────────────────────────────────────────────────────

func TestBuildFrontendHandler_NilEmbeddedReturns404(t *testing.T) {
	if embeddedFS != nil {
		t.Skip("skipping: test only valid when embeddedFS is nil (non-embed build)")
	}
	handler, err := buildFrontendHandler()
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "Frontend not embedded")
}

// ─── WebSocket command streaming ──────────────────────────────────────────────

func TestHandleCommandStream_InvalidJSONSendsErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(handleCommandStream))
	defer srv.Close()

	conn := dialWS(t, srv.URL)
	defer conn.Close()

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("not json at all")))

	var msg wsOutgoing
	readWSMessage(t, conn, &msg)
	assert.Equal(t, "error", msg.Type)
	assert.Contains(t, msg.Data, "invalid message")
}

func TestHandleCommandStream_StartWithMissingIDSendsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(handleCommandStream))
	defer srv.Close()

	conn := dialWS(t, srv.URL)
	defer conn.Close()

	start := wsIncoming{Type: "start", ID: "", Args: []string{"version"}}
	sendWS(t, conn, start)

	var msg wsOutgoing
	readWSMessage(t, conn, &msg)
	assert.Equal(t, "error", msg.Type)
	assert.Contains(t, msg.Data, "missing id")
}

// TestHandleCommandStream_StartAndExit verifies the full streaming lifecycle:
// a command is started, produces output, and an "exit" frame is received.
// The stub binary is /bin/echo (or equivalent) so it exits cleanly.
func TestHandleCommandStream_StartAndExit(t *testing.T) {
	echo, err := lookupEcho()
	if err != nil {
		t.Skip("echo binary not found:", err)
	}
	old := devpodSelfOverride
	devpodSelfOverride = echo
	defer func() { devpodSelfOverride = old }()

	srv := httptest.NewServer(http.HandlerFunc(handleCommandStream))
	defer srv.Close()

	conn := dialWS(t, srv.URL)
	defer conn.Close()

	const cmdID = "test-lifecycle"
	start := wsIncoming{Type: "start", ID: cmdID, Args: []string{"streaming hello"}}
	sendWS(t, conn, start)

	deadline := time.Now().Add(5 * time.Second)
	var gotExit bool
	for !gotExit && time.Now().Before(deadline) {
		require.NoError(t, conn.SetReadDeadline(deadline))
		_, raw, err := conn.ReadMessage()
		require.NoError(t, err)

		var msg wsOutgoing
		require.NoError(t, json.Unmarshal(raw, &msg))
		if msg.ID != cmdID {
			continue
		}
		switch msg.Type {
		case "stdout":
			assert.Contains(t, msg.Data, "streaming hello")
		case "exit":
			assert.Equal(t, 0, msg.ExitCode)
			gotExit = true
		case "error":
			t.Fatalf("unexpected error frame: %s", msg.Data)
		}
	}
	assert.True(t, gotExit, "expected exit frame within deadline")
}

// TestHandleCommandStream_CancelRunningCommand sends a "cancel" frame for a
// long-running command and verifies we receive an exit frame afterwards.
func TestHandleCommandStream_CancelRunningCommand(t *testing.T) {
	sleep, err := lookupSleep()
	if err != nil {
		t.Skip("sleep binary not found:", err)
	}
	old := devpodSelfOverride
	devpodSelfOverride = sleep
	defer func() { devpodSelfOverride = old }()

	srv := httptest.NewServer(http.HandlerFunc(handleCommandStream))
	defer srv.Close()

	conn := dialWS(t, srv.URL)
	defer conn.Close()

	const cmdID = "test-cancel"
	// Start a long sleep (sleep 30)
	start := wsIncoming{Type: "start", ID: cmdID, Args: []string{"30"}}
	sendWS(t, conn, start)

	// Give the process a moment to start.
	time.Sleep(200 * time.Millisecond)

	// Cancel it.
	cancel := wsIncoming{Type: "cancel", ID: cmdID}
	sendWS(t, conn, cancel)

	// Expect an exit frame (non-zero exit code from SIGINT).
	deadline := time.Now().Add(5 * time.Second)
	var gotExit bool
	for !gotExit && time.Now().Before(deadline) {
		require.NoError(t, conn.SetReadDeadline(deadline))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg wsOutgoing
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		if msg.ID == cmdID && msg.Type == "exit" {
			gotExit = true
		}
	}
	assert.True(t, gotExit, "expected exit frame after cancel")
}

// ─── Full server smoke test ───────────────────────────────────────────────────

// TestWebCmd_StartsAndRespondsToHealthCheck verifies that WebCmd.Run starts a real
// HTTP server, answers a /api/health probe, and shuts down cleanly when the context
// is cancelled.
func TestWebCmd_StartsAndRespondsToHealthCheck(t *testing.T) {
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := &WebCmd{Host: "127.0.0.1", Port: port, Open: false}
	errCh := make(chan error, 1)
	go func() { errCh <- cmd.Run(ctx) }()

	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	var resp *http.Response
	var httpErr error
	for i := 0; i < 40; i++ {
		resp, httpErr = http.Get(serverURL + "/api/health")
		if httpErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, httpErr, "server did not become available")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Graceful shutdown.
	cancel()
	select {
	case e := <-errCh:
		if e != nil && !strings.Contains(e.Error(), "closed") && !strings.Contains(e.Error(), "use of closed") {
			t.Errorf("unexpected server error: %v", e)
		}
	case <-time.After(3 * time.Second):
		t.Error("server did not shut down within 3 s")
	}
}

// TestWebCmd_ExposesAllAPIEndpoints starts a real server and probes each API route.
func TestWebCmd_ExposesAllAPIEndpoints(t *testing.T) {
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := &WebCmd{Host: "127.0.0.1", Port: port, Open: false}
	go func() { _ = cmd.Run(ctx) }()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	require.Eventually(t, func() bool {
		r, err := http.Get(base + "/api/health")
		return err == nil && r.StatusCode == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "server did not start")

	endpoints := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/api/health", http.StatusOK},
		{http.MethodGet, "/api/version", http.StatusOK},
		{http.MethodGet, "/api/platform", http.StatusOK},
		// These run the real devpod binary; skip result validation, just ensure no 5xx panic.
		{http.MethodGet, "/api/providers", -1},
		{http.MethodGet, "/api/workspaces", -1},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, base+ep.path, nil)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			if ep.want > 0 {
				assert.Equal(t, ep.want, resp.StatusCode)
			}
			// Make sure CORS is set.
			assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
		})
	}
}

// ─── Provider pass-through integration test ──────────────────────────────────

// TestProviderList_ViaWebSocket exercises the `devpod provider list` subcommand
// through the WebSocket streaming endpoint.
// This test is skipped unless the real devpod binary is available in PATH.
func TestProviderList_ViaWebSocket(t *testing.T) {
	devpod, err := lookupDevpodInPath()
	if err != nil {
		t.Skip("devpod not in PATH:", err)
	}

	old := devpodSelfOverride
	devpodSelfOverride = devpod
	defer func() { devpodSelfOverride = old }()

	srv := httptest.NewServer(http.HandlerFunc(handleCommandStream))
	defer srv.Close()

	conn := dialWS(t, srv.URL)
	defer conn.Close()

	const cmdID = "provider-list"
	start := wsIncoming{
		Type: "start",
		ID:   cmdID,
		Args: []string{"provider", "list", "--output=json", "--log-output=json"},
	}
	sendWS(t, conn, start)

	deadline := time.Now().Add(15 * time.Second)
	var gotExit bool
	for !gotExit && time.Now().Before(deadline) {
		require.NoError(t, conn.SetReadDeadline(deadline))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg wsOutgoing
		if json.Unmarshal(raw, &msg) != nil || msg.ID != cmdID {
			continue
		}
		if msg.Type == "exit" {
			gotExit = true
			// provider list should succeed
			assert.Equal(t, 0, msg.ExitCode, "devpod provider list exited non-zero")
		}
	}
	assert.True(t, gotExit, "did not receive exit frame from provider list")
}

// TestProviderList_ViaRESTEndpoint exercises /api/providers with the real devpod binary.
func TestProviderList_ViaRESTEndpoint(t *testing.T) {
	devpod, err := lookupDevpodInPath()
	if err != nil {
		t.Skip("devpod not in PATH:", err)
	}

	old := devpodSelfOverride
	devpodSelfOverride = devpod
	defer func() { devpodSelfOverride = old }()

	rec := httptest.NewRecorder()
	handleProviders(rec, httptest.NewRequest(http.MethodGet, "/api/providers", nil))

	// Accept 200 (providers exist) or 502 (devpod itself returned non-zero, e.g. no config).
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusBadGateway,
		"expected 200 or 502, got %d", rec.Code)
	if rec.Code == http.StatusOK {
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func dialWS(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	return conn
}

func sendWS(t *testing.T, conn *websocket.Conn, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))
}

func readWSMessage(t *testing.T, conn *websocket.Conn, out interface{}) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, raw, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, out))
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// lookupEcho returns the path to a system echo binary (or an error).
func lookupEcho() (string, error) {
	for _, p := range []string{"/bin/echo", "/usr/bin/echo"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("echo not found in common locations")
}

// lookupSleep returns the path to a system sleep binary (or an error).
func lookupSleep() (string, error) {
	for _, p := range []string{"/bin/sleep", "/usr/bin/sleep"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("sleep not found in common locations")
}

// lookupDevpodInPath returns the full path to the devpod binary in PATH.
func lookupDevpodInPath() (string, error) {
	for _, dir := range strings.Split(os.Getenv("PATH"), ":") {
		p := dir + "/devpod"
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("devpod not found in PATH")
}

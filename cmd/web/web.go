package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/skevetter/devpod/cmd/flags"
	"github.com/spf13/cobra"
)

// WebCmd holds the configuration for the web command
type WebCmd struct {
	globalFlags *flags.GlobalFlags
	Host        string
	Port        int
	Open        bool
}

// NewWebCmd creates the `devpod web` sub-command
func NewWebCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &WebCmd{globalFlags: flags}
	webCmd := &cobra.Command{
		Use:   "web",
		Short: "Start the DevPod web UI in your browser",
		Long: `Start a local HTTP server that serves the DevPod web UI.
This allows you to use DevPod from any browser without installing the desktop app.

Example:
  devpod web
  devpod web --port 8090
  devpod web --host 0.0.0.0 --port 8090`,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd.Context())
		},
	}

	webCmd.Flags().StringVar(&cmd.Host, "host", "127.0.0.1", "Host address to bind the web server to")
	webCmd.Flags().IntVar(&cmd.Port, "port", 8090, "Port to run the web server on")
	webCmd.Flags().BoolVar(&cmd.Open, "open", true, "Automatically open the browser")

	return webCmd
}

func (cmd *WebCmd) Run(ctx context.Context) error {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/version", handleVersion)
	mux.HandleFunc("/api/platform", handlePlatform)
	mux.HandleFunc("/api/command", handleCommandRun)
	mux.HandleFunc("/api/command/stream", handleCommandStream)
	mux.HandleFunc("/api/signal", handleSignal)
	mux.HandleFunc("/api/providers", handleProviders)
	mux.HandleFunc("/api/workspaces", handleWorkspaces)
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/releases", handleReleases)

	// Serve the frontend - use embedded files if available, otherwise serve from local dist
	frontendHandler, err := buildFrontendHandler()
	if err != nil {
		return fmt.Errorf("failed to set up frontend: %w", err)
	}
	mux.Handle("/", frontendHandler)

	addr := fmt.Sprintf("%s:%d", cmd.Host, cmd.Port)
	url := fmt.Sprintf("http://%s:%d", cmd.Host, cmd.Port)

	// Check if the port is available
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot bind to %s: %w", addr, err)
	}

	log.Printf("DevPod web UI available at %s", url)

	if cmd.Open {
		go openBrowser(url)
	}

	server := &http.Server{Handler: withCORS(mux)}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	return server.Serve(listener)
}

// withCORS adds CORS headers so the frontend can talk to the API even during development
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// buildFrontendHandler returns an http.Handler that serves the embedded web frontend.
// The embedded FS is set by the build tag in frontend_embed.go (production) or
// frontend_dev.go (development / no embed).
func buildFrontendHandler() (http.Handler, error) {
	if embeddedFS == nil {
		// Development mode: serve from desktop/dist relative to the binary
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Frontend not embedded. Build with: make build-web", http.StatusNotFound)
		}), nil
	}

	sub, err := fs.Sub(embeddedFS, "dist")
	if err != nil {
		return nil, err
	}

	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For SPA routing: serve index.html for paths that don't match a file
		_, statErr := fs.Stat(sub, strings.TrimPrefix(r.URL.Path, "/"))
		if statErr != nil && r.URL.Path != "/" {
			// Serve index.html to let the frontend router handle the path
			r2 := *r
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, &r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}

// devpodSelf returns the path to the running devpod binary.
// devpodSelfOverride can be set in tests to inject a stub binary.
var devpodSelfOverride string

func devpodSelf() (string, error) {
	if devpodSelfOverride != "" {
		return devpodSelfOverride, nil
	}
	return os.Executable()
}

// ---- API handlers ----

func handleVersion(w http.ResponseWriter, r *http.Request) {
	version := os.Getenv("DEVPOD_VERSION")
	if version == "" {
		version = "dev"
	}
	writeJSON(w, map[string]string{"version": version})
}

func handlePlatform(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{
		"platform": runtime.GOOS,
		"arch":     runtime.GOARCH,
	})
}

// ---- Synchronous command execution ----

type commandRequest struct {
	Args []string          `json:"args"`
	Env  map[string]string `json:"env"`
}

type commandResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

func handleCommandRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	self, err := devpodSelf()
	if err != nil {
		http.Error(w, "cannot resolve devpod binary", http.StatusInternalServerError)
		return
	}

	cmd := exec.CommandContext(r.Context(), self, req.Args...)
	cmd.Env = buildEnv(req.Env)

	stdout, cmdErr := cmd.Output()
	exitCode := 0
	var stderr string
	if exitErr, ok := cmdErr.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
		stderr = string(exitErr.Stderr)
	}

	writeJSON(w, commandResponse{
		Stdout:   string(stdout),
		Stderr:   stderr,
		ExitCode: exitCode,
	})
}

// ---- Streaming command execution over WebSocket ----

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsIncoming struct {
	Type string            `json:"type"` // "start" | "cancel"
	ID   string            `json:"id"`
	Args []string          `json:"args"`
	Env  map[string]string `json:"env"`
}

type wsOutgoing struct {
	Type     string `json:"type"` // "stdout" | "stderr" | "exit" | "error"
	ID       string `json:"id"`
	Data     string `json:"data,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
}

func handleCommandStream(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Track running processes so we can cancel them
	var mu sync.Mutex
	processes := map[string]*exec.Cmd{}

	writeMu := sync.Mutex{}
	send := func(msg wsOutgoing) {
		data, _ := json.Marshal(msg)
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.WriteMessage(websocket.TextMessage, data)
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg wsIncoming
		if err := json.Unmarshal(raw, &msg); err != nil {
			send(wsOutgoing{Type: "error", Data: "invalid message: " + err.Error()})
			continue
		}

		switch msg.Type {
		case "start":
			if msg.ID == "" {
				send(wsOutgoing{Type: "error", Data: "missing id"})
				continue
			}

			go func(id string, args []string, env map[string]string) {
				self, err := devpodSelf()
				if err != nil {
					send(wsOutgoing{Type: "error", ID: id, Data: "cannot resolve devpod binary"})
					return
				}

				cmd := exec.Command(self, args...)
				cmd.Env = buildEnv(env)

				stdoutPipe, _ := cmd.StdoutPipe()
				stderrPipe, _ := cmd.StderrPipe()

				if err := cmd.Start(); err != nil {
					send(wsOutgoing{Type: "error", ID: id, Data: err.Error()})
					return
				}

				mu.Lock()
				processes[id] = cmd
				mu.Unlock()

				var wg sync.WaitGroup
				wg.Add(2)

				go func() {
					defer wg.Done()
					buf := make([]byte, 4096)
					for {
						n, err := stdoutPipe.Read(buf)
						if n > 0 {
							send(wsOutgoing{Type: "stdout", ID: id, Data: string(buf[:n])})
						}
						if err != nil {
							break
						}
					}
				}()

				go func() {
					defer wg.Done()
					buf := make([]byte, 4096)
					for {
						n, err := stderrPipe.Read(buf)
						if n > 0 {
							send(wsOutgoing{Type: "stderr", ID: id, Data: string(buf[:n])})
						}
						if err != nil {
							break
						}
					}
				}()

				wg.Wait()
				exitCode := 0
				if err := cmd.Wait(); err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						exitCode = exitErr.ExitCode()
					}
				}

				mu.Lock()
				delete(processes, id)
				mu.Unlock()

				send(wsOutgoing{Type: "exit", ID: id, ExitCode: exitCode})
			}(msg.ID, msg.Args, msg.Env)

		case "cancel":
			mu.Lock()
			if cmd, ok := processes[msg.ID]; ok {
				_ = cmd.Process.Signal(os.Interrupt)
			}
			mu.Unlock()
		}
	}
}

// ---- Signal handler ----

type signalRequest struct {
	ProcessID int `json:"processId"`
	Signal    int `json:"signal"`
}

func handleSignal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req signalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	p, err := os.FindProcess(req.ProcessID)
	if err != nil {
		http.Error(w, "process not found", http.StatusNotFound)
		return
	}

	_ = p.Signal(os.Interrupt)
	w.WriteHeader(http.StatusOK)
}

// ---- High-level convenience endpoints ----

// handleHealth returns 200 OK so load-balancers / health checks can probe the server.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleReleases returns an empty JSON array. The desktop app fetches release
// info from the Tauri server; in web mode we don't have that, so we return an
// empty list to prevent JSON parse errors in the frontend.
func handleReleases(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("[]"))
}

// handleProviders runs `devpod provider list --output=json` and proxies the JSON response.
// This is a convenience endpoint so the frontend can fetch providers without opening a WebSocket.
func handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runJSONSubcommand(w, []string{"provider", "list", "--output=json", "--log-output=json"})
}

// handleWorkspaces runs `devpod list --output=json` and proxies the JSON response.
func handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runJSONSubcommand(w, []string{"list", "--output=json", "--log-output=json"})
}

// runJSONSubcommand executes the devpod binary with the given args and streams the raw stdout
// directly to the response writer. Stderr is discarded; on failure a 502 is returned.
func runJSONSubcommand(w http.ResponseWriter, args []string) {
	self, err := devpodSelf()
	if err != nil {
		http.Error(w, "cannot resolve devpod binary: "+err.Error(), http.StatusInternalServerError)
		return
	}

	out, err := exec.Command(self, args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			http.Error(w, string(ee.Stderr), http.StatusBadGateway)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

// ---- Utilities ----

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON error: %v", err)
	}
}

func buildEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux, freebsd, etc.
		cmd = "xdg-open"
		args = []string{url}
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}

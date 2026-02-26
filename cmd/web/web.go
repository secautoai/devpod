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
	"github.com/skevetter/devpod/cmd/web/db"
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
	dataDir := userDataDir()

	// Initialise persistent stores
	database, err := db.Open(dataDir)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Initialise JWT secret (persisted across restarts)
	if err := initJWTSecret(dataDir); err != nil {
		return fmt.Errorf("init JWT secret: %w", err)
	}

	mux := http.NewServeMux()

	// ── Public endpoints (no auth required) ──────────────────────────────────
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/version", handleVersion)
	mux.HandleFunc("/api/platform", handlePlatform)
	mux.HandleFunc("/releases", handleReleases)
	mux.HandleFunc("/api/branding", makeBrandingGetHandler(database.Branding))
	mux.HandleFunc("/api/login", makeLoginHandler(database.Users))

	// ── Authenticated endpoints ───────────────────────────────────────────────
	mux.HandleFunc("/api/me", makeMeHandler(database.Users))
	mux.HandleFunc("/api/command", withAuth(makeCommandRunHandler(database)))
	mux.HandleFunc("/api/command/stream", makeCommandStreamHandler(database))
	mux.HandleFunc("/api/signal", withAuth(handleSignal))
	mux.HandleFunc("/api/providers", withAuth(makeProvidersHandler(database)))
	mux.HandleFunc("/api/workspaces", withAuth(makeWorkspacesHandler(database)))
	mux.HandleFunc("/api/logs", withAuth(makeUserLogsHandler(database.OpLog)))

	// ── Admin-only endpoints ──────────────────────────────────────────────────
	// Users
	mux.HandleFunc("/api/admin/users", withAdmin(makeAdminUsersHandler(database)))
	mux.HandleFunc("/api/admin/users/", withAdmin(makeAdminUserDetailHandler(database)))
	// Workspaces across all users
	mux.HandleFunc("/api/admin/workspaces", makeAdminWorkspacesHandler(database.Users))
	// Operation logs
	mux.HandleFunc("/api/admin/logs", withAdmin(makeAdminLogsHandler(database.OpLog)))
	// Branding
	mux.HandleFunc("/api/admin/branding", withAdmin(makeBrandingUpdateHandler(database.Branding)))
	// Password change (self-service)
	mux.HandleFunc("/api/user/password", withAuth(makeChangePasswordHandler(database.Users)))

	// ── Frontend ──────────────────────────────────────────────────────────────
	frontendHandler, err := buildFrontendHandler()
	if err != nil {
		return fmt.Errorf("failed to set up frontend: %w", err)
	}
	mux.Handle("/", frontendHandler)

	addr := fmt.Sprintf("%s:%d", cmd.Host, cmd.Port)
	url := fmt.Sprintf("http://%s:%d", cmd.Host, cmd.Port)

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

// ─── CORS ─────────────────────────────────────────────────────────────────────

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

// ─── Frontend handler ─────────────────────────────────────────────────────────

func buildFrontendHandler() (http.Handler, error) {
	if embeddedFS == nil {
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
		_, statErr := fs.Stat(sub, strings.TrimPrefix(r.URL.Path, "/"))
		if statErr != nil && r.URL.Path != "/" {
			r2 := *r
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, &r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}

// ─── devpod binary resolution ─────────────────────────────────────────────────

var devpodSelfOverride string

func devpodSelf() (string, error) {
	if devpodSelfOverride != "" {
		return devpodSelfOverride, nil
	}
	return os.Executable()
}

// ─── Public API handlers ──────────────────────────────────────────────────────

func handleVersion(w http.ResponseWriter, _ *http.Request) {
	version := os.Getenv("DEVPOD_VERSION")
	if version == "" {
		version = "dev"
	}
	writeJSON(w, map[string]string{"version": version})
}

func handlePlatform(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"platform": runtime.GOOS, "arch": runtime.GOARCH})
}

func handleHealth(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func handleReleases(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("[]"))
}

// ─── Synchronous command execution ───────────────────────────────────────────

type commandRequest struct {
	Args []string          `json:"args"`
	Env  map[string]string `json:"env"`
}

type commandResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"code"`
}

func makeCommandRunHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req commandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		claims := getClaims(r)
		username := ""
		if claims != nil {
			username = claims.Username
		}

		// RBAC: check if the user is allowed to use the requested provider
		if providerID := extractProvider(req.Args); providerID != "" && username != "" {
			if !database.Permissions.IsAllowed(username, providerID) {
				http.Error(w, fmt.Sprintf("provider %q is not allowed for your account", providerID), http.StatusForbidden)
				return
			}
		}

		self, err := devpodSelf()
		if err != nil {
			http.Error(w, "cannot resolve devpod binary", http.StatusInternalServerError)
			return
		}

		// Log the operation asynchronously
		action := inferAction(req.Args)
		target := inferTarget(req.Args)
		logID, _ := database.OpLog.Append(username, action, target, req.Args)

		// Build user-isolated environment
		var dockerHost string
		if claims != nil {
			if u, ok := database.Users.GetUser(claims.Username); ok {
				dockerHost = u.DockerHost
			}
		}
		devpodHome := userDevpodHome(username)
		_ = ensureUserHome(username)

		cmd := exec.CommandContext(r.Context(), self, req.Args...)
		cmd.Env = buildUserEnv(username, devpodHome, dockerHost, req.Env)

		stdout, cmdErr := cmd.Output()
		exitCode := 0
		var stderr string
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			stderr = string(exitErr.Stderr)
		}

		// Finish the log entry
		if logID != "" {
			errMsg := ""
			if cmdErr != nil && exitCode != 0 {
				errMsg = cmdErr.Error()
			}
			go func() { _ = database.OpLog.Finish(logID, exitCode, errMsg) }()
		}

		writeJSON(w, commandResponse{Stdout: string(stdout), Stderr: stderr, ExitCode: exitCode})
	}
}

// ─── Streaming command execution over WebSocket ───────────────────────────────

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

func makeCommandStreamHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// WebSocket cannot set headers, so accept token as query param
		token := r.URL.Query().Get("token")
		var username, dockerHost string
		if token != "" {
			if claims, err := validateToken(token); err == nil {
				username = claims.Username
				if u, ok := database.Users.GetUser(claims.Username); ok {
					dockerHost = u.DockerHost
				}
			}
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

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

				// RBAC check
				if providerID := extractProvider(msg.Args); providerID != "" && username != "" {
					if !database.Permissions.IsAllowed(username, providerID) {
						send(wsOutgoing{Type: "error", ID: msg.ID, Data: fmt.Sprintf("provider %q is not allowed for your account", providerID)})
						continue
					}
				}

				go func(id string, args []string, env map[string]string) {
					self, err := devpodSelf()
					if err != nil {
						send(wsOutgoing{Type: "error", ID: id, Data: "cannot resolve devpod binary"})
						return
					}

					devpodHome := userDevpodHome(username)
					_ = ensureUserHome(username)

					action := inferAction(args)
					target := inferTarget(args)
					logID, _ := database.OpLog.Append(username, action, target, args)

					cmd := exec.Command(self, args...)
					cmd.Env = buildUserEnv(username, devpodHome, dockerHost, env)

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
					errMsg := ""
					if err := cmd.Wait(); err != nil {
						if exitErr, ok := err.(*exec.ExitError); ok {
							exitCode = exitErr.ExitCode()
						}
						errMsg = err.Error()
					}

					mu.Lock()
					delete(processes, id)
					mu.Unlock()

					if logID != "" {
						_ = database.OpLog.Finish(logID, exitCode, errMsg)
					}
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
}

// ─── Signal handler ───────────────────────────────────────────────────────────

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

// ─── Providers / Workspaces (user-scoped) ─────────────────────────────────────

func makeProvidersHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		claims := getClaims(r)
		username := ""
		if claims != nil {
			username = claims.Username
		}
		runUserJSONSubcommand(w, username, database, []string{"provider", "list", "--output=json", "--log-output=json"})
	}
}

func makeWorkspacesHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		claims := getClaims(r)
		username := ""
		if claims != nil {
			username = claims.Username
		}
		runUserJSONSubcommand(w, username, database, []string{"list", "--output=json", "--log-output=json"})
	}
}

func runUserJSONSubcommand(w http.ResponseWriter, username string, database *db.DB, args []string) {
	self, err := devpodSelf()
	if err != nil {
		http.Error(w, "cannot resolve devpod binary: "+err.Error(), http.StatusInternalServerError)
		return
	}
	dockerHost := ""
	if username != "" {
		if u, ok := database.Users.GetUser(username); ok {
			dockerHost = u.DockerHost
		}
	}
	devpodHome := userDevpodHome(username)
	_ = ensureUserHome(username)

	cmd := buildUserCommand(self, username, devpodHome, dockerHost, args)
	out, err := cmd.Output()
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

// ─── Admin: users CRUD + provider permissions ─────────────────────────────────

func makeAdminUsersHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, database.Users.ListUsers())
		case http.MethodPost:
			var req createUserRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if req.Username == "" || req.Password == "" {
				http.Error(w, "username and password are required", http.StatusBadRequest)
				return
			}
			if req.Role == "" {
				req.Role = db.RoleUser
			}
			if req.Role != db.RoleAdmin && req.Role != db.RoleUser {
				http.Error(w, "role must be 'admin' or 'user'", http.StatusBadRequest)
				return
			}
			user, err := database.Users.CreateUser(req.Username, req.Password, req.Role)
			if err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, user.Public())
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// makeAdminUserDetailHandler handles /api/admin/users/{username} and
// /api/admin/users/{username}/providers
func makeAdminUserDetailHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Strip prefix and split path segments
		path := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
		parts := strings.SplitN(path, "/", 2)
		username := parts[0]
		sub := ""
		if len(parts) == 2 {
			sub = parts[1]
		}

		if username == "" {
			http.Error(w, "username required", http.StatusBadRequest)
			return
		}

		switch sub {
		case "":
			// /api/admin/users/{username}
			switch r.Method {
			case http.MethodPut:
				var req updateUserRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid request body", http.StatusBadRequest)
					return
				}
				if req.Role != nil && *req.Role != db.RoleAdmin && *req.Role != db.RoleUser {
					http.Error(w, "role must be 'admin' or 'user'", http.StatusBadRequest)
					return
				}
				user, err := database.Users.UpdateUser(username, req.Role, req.DockerHost)
				if err != nil {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				writeJSON(w, user.Public())

			case http.MethodDelete:
				if err := database.Users.DeleteUser(username); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				_ = database.Permissions.DeleteUser(username)
				w.WriteHeader(http.StatusNoContent)

			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}

		case "providers":
			// /api/admin/users/{username}/providers
			switch r.Method {
			case http.MethodGet:
				allowed, restricted := database.Permissions.GetAllowed(username)
				if !restricted {
					writeJSON(w, map[string]interface{}{"unrestricted": true, "providers": nil})
				} else {
					writeJSON(w, map[string]interface{}{"unrestricted": false, "providers": allowed})
				}

			case http.MethodPut:
				var req struct {
					Providers *[]string `json:"providers"` // null = unrestricted
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid request body", http.StatusBadRequest)
					return
				}
				if err := database.Permissions.SetAllowed(username, req.Providers); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)

			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}
}

func makeAdminWorkspacesHandler(users *db.UserStore) http.HandlerFunc {
	return withAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		type userWorkspaces struct {
			Username   string          `json:"username"`
			Workspaces json.RawMessage `json:"workspaces"`
		}
		var results []userWorkspaces
		for _, u := range users.ListUsers() {
			homeDir := userDevpodHome(u.Username)
			self, err := devpodSelf()
			if err != nil {
				continue
			}
			cmd := buildUserCommand(self, u.Username, homeDir, u.DockerHost, []string{"list", "--output=json", "--log-output=json"})
			out, err := cmd.Output()
			if err != nil {
				results = append(results, userWorkspaces{Username: u.Username, Workspaces: json.RawMessage("[]")})
				continue
			}
			results = append(results, userWorkspaces{Username: u.Username, Workspaces: json.RawMessage(out)})
		}
		writeJSON(w, results)
	})
}

// ─── Operation logs ───────────────────────────────────────────────────────────

func makeUserLogsHandler(oplog *db.OpLogStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		claims := getClaims(r)
		username := ""
		if claims != nil {
			username = claims.Username
		}
		f := parseLogFilter(r, username)
		writeJSON(w, database_safeEntries(oplog.Query(f)))
	}
}

func makeAdminLogsHandler(oplog *db.OpLogStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Admin can filter by any username
		f := parseLogFilter(r, r.URL.Query().Get("username"))
		writeJSON(w, database_safeEntries(oplog.Query(f)))
	}
}

func parseLogFilter(r *http.Request, username string) db.OpLogFilter {
	q := r.URL.Query()
	limit := 50
	offset := 0
	fmt.Sscanf(q.Get("limit"), "%d", &limit)
	fmt.Sscanf(q.Get("offset"), "%d", &offset)
	return db.OpLogFilter{
		Username: username,
		Action:   q.Get("action"),
		Limit:    limit,
		Offset:   offset,
	}
}

// database_safeEntries returns an empty slice instead of nil for JSON encoding.
func database_safeEntries(entries []*db.OperationLog) []*db.OperationLog {
	if entries == nil {
		return []*db.OperationLog{}
	}
	return entries
}

// ─── Branding ─────────────────────────────────────────────────────────────────

func makeBrandingGetHandler(branding *db.BrandingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, branding.Get())
	}
}

func makeBrandingUpdateHandler(branding *db.BrandingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			var partial db.BrandingSettings
			if err := json.NewDecoder(r.Body).Decode(&partial); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			if err := branding.Update(partial); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, branding.Get())
		case http.MethodDelete:
			if err := branding.Reset(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, branding.Get())
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ─── Password change (self-service) ──────────────────────────────────────────

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func makeChangePasswordHandler(users *db.UserStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		claims := getClaims(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req changePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if _, err := users.Authenticate(claims.Username, req.CurrentPassword); err != nil {
			http.Error(w, "current password is incorrect", http.StatusUnauthorized)
			return
		}
		if len(req.NewPassword) < 6 {
			http.Error(w, "new password must be at least 6 characters", http.StatusBadRequest)
			return
		}
		if err := users.ChangePassword(claims.Username, req.NewPassword); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── RBAC helpers ─────────────────────────────────────────────────────────────

// extractProvider parses --provider=<id> or -p <id> from a devpod args slice.
func extractProvider(args []string) string {
	for i, arg := range args {
		if strings.HasPrefix(arg, "--provider=") {
			return strings.TrimPrefix(arg, "--provider=")
		}
		if (arg == "--provider" || arg == "-p") && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// inferAction derives a human-readable action name from args.
func inferAction(args []string) string {
	for _, a := range args {
		switch a {
		case "create", "delete", "start", "stop", "status", "list", "up", "down":
			return a
		}
	}
	if len(args) > 0 {
		return args[0]
	}
	return "command"
}

// inferTarget extracts the workspace or provider name from args (typically the
// first non-flag argument after the subcommand).
func inferTarget(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

// ─── Request/response types used across handlers ─────────────────────────────

type createUserRequest struct {
	Username string      `json:"username"`
	Password string      `json:"password"`
	Role     db.UserRole `json:"role"`
}

type updateUserRequest struct {
	Role       *db.UserRole `json:"role,omitempty"`
	DockerHost *string      `json:"dockerHost,omitempty"`
}

// ─── Utilities ────────────────────────────────────────────────────────────────

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
	var cmdArgs []string
	switch runtime.GOOS {
	case "windows":
		cmd, cmdArgs = "cmd", []string{"/c", "start", url}
	case "darwin":
		cmd, cmdArgs = "open", []string{url}
	default:
		cmd, cmdArgs = "xdg-open", []string{url}
	}
	if err := exec.Command(cmd, cmdArgs...).Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}

package web

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

// userDataDir returns the base directory where per-user data is stored.
// By default this is ~/.devpod-server/data, but can be overridden with
// DEVPOD_WEB_DATA_DIR.
func userDataDir() string {
	if d := os.Getenv("DEVPOD_WEB_DATA_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/var/lib/devpod-web"
	}
	return filepath.Join(home, ".devpod-server", "data")
}

// userDevpodHome returns the DEVPOD_HOME directory for a specific user.
// Each user gets an isolated config directory so their providers, workspaces,
// and credentials are separate.
func userDevpodHome(username string) string {
	return filepath.Join(userDataDir(), "users", username, ".devpod")
}

// ensureUserHome creates the per-user DEVPOD_HOME directory if it doesn't exist.
func ensureUserHome(username string) error {
	return os.MkdirAll(userDevpodHome(username), 0700)
}

// buildUserCommand creates an exec.Cmd that runs a devpod subcommand in the
// context of a specific user's DEVPOD_HOME. If dockerHost is non-empty, it also
// sets DOCKER_HOST so the user's containers are isolated.
func buildUserCommand(devpodBin, username, devpodHome, dockerHost string, args []string) *exec.Cmd {
	cmd := exec.Command(devpodBin, args...)
	cmd.Env = buildUserEnv(username, devpodHome, dockerHost, nil)
	return cmd
}

// buildUserCommandContext is like buildUserCommand but accepts a context.
func buildUserCommandContext(ctx context.Context, devpodBin, username, devpodHome, dockerHost string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, devpodBin, args...)
	cmd.Env = buildUserEnv(username, devpodHome, dockerHost, nil)
	return cmd
}

// buildUserEnv constructs the environment for a user's devpod process.
// It sets DEVPOD_HOME and optionally DOCKER_HOST, then merges any extra
// env vars from the request.
func buildUserEnv(username, devpodHome, dockerHost string, extra map[string]string) []string {
	env := os.Environ()
	env = append(env, "DEVPOD_HOME="+devpodHome)
	env = append(env, "DEVPOD_USER="+username)
	if dockerHost != "" {
		env = append(env, "DOCKER_HOST="+dockerHost)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

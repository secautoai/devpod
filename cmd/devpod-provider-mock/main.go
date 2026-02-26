// devpod-provider-mock is a minimal DevPod provider for testing.
// It stores workspace state as JSON files under ~/.devpod-mock/workspaces/
// and requires no Docker, SSH, or network access.
//
// Build: go build -o devpod-provider-mock ./cmd/devpod-provider-mock/
// Install: cp devpod-provider-mock ~/.local/bin/
//
// Add to devpod:
//   devpod provider add ./provider.yaml --name mock
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "devpod-provider-mock",
		Short: "Mock DevPod provider for testing",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	root.AddCommand(
		cmdInit(),
		cmdCreate(),
		cmdStart(),
		cmdStop(),
		cmdDelete(),
		cmdStatus(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func cmdInit() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialise the mock provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := os.MkdirAll(stateDir(), 0700); err != nil {
				return err
			}
			logLine("info", "Mock provider initialised at "+stateDir())
			return nil
		},
	}
}

func cmdCreate() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			id := workspaceID()
			logLine("info", fmt.Sprintf("Creating workspace %q", id))
			r := &WorkspaceRecord{ID: id, State: StateStopped}
			if err := persist(r); err != nil {
				return err
			}
			logLine("info", fmt.Sprintf("Workspace %q created", id))
			return nil
		},
	}
}

func cmdStart() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			id := workspaceID()
			r, err := load(id)
			if err != nil {
				return err
			}
			logLine("info", fmt.Sprintf("Starting workspace %q", id))
			r.State = StateRunning
			if err := persist(r); err != nil {
				return err
			}
			logLine("info", fmt.Sprintf("Workspace %q is running", id))
			return nil
		},
	}
}

func cmdStop() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			id := workspaceID()
			r, err := load(id)
			if err != nil {
				return err
			}
			logLine("info", fmt.Sprintf("Stopping workspace %q", id))
			r.State = StateStopped
			if err := persist(r); err != nil {
				return err
			}
			logLine("info", fmt.Sprintf("Workspace %q stopped", id))
			return nil
		},
	}
}

func cmdDelete() *cobra.Command {
	return &cobra.Command{
		Use:   "delete",
		Short: "Delete a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			id := workspaceID()
			logLine("info", fmt.Sprintf("Deleting workspace %q", id))
			if err := remove(id); err != nil {
				return err
			}
			logLine("info", fmt.Sprintf("Workspace %q deleted", id))
			return nil
		},
	}
}

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print workspace status as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			id := workspaceID()
			r, err := load(id)
			if err != nil {
				return err
			}
			out, err := json.Marshal(map[string]string{
				"id":    r.ID,
				"state": string(r.State),
			})
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			return nil
		},
	}
}

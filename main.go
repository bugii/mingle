package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type Session struct {
	Name       string
	Path       string
	Type       string
	Tmuxinator string
}

type ConfigSession struct {
	Type       *string `yaml:"type"`
	Path       string  `yaml:"path"`
	Tmuxinator string  `yaml:"tmuxinator,omitempty"`
}

const configPath = ".config/mingle/mingle.yaml"

func getConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	return filepath.Join(homeDir, configPath)
}

func loadConfig() ([]ConfigSession, error) {
	filePath := getConfigPath()
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Println("No config file was found")
		return []ConfigSession{}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config []ConfigSession
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	for i := range config {
		expandedPath, err := expandHomePath(config[i].Path)
		if err != nil {
			return nil, err
		}
		config[i].Path = expandedPath
	}

	return config, nil
}

func getSessions() ([]Session, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	tmuxSessions, err := getTmuxSessions()
	if err != nil {
		return nil, err
	}

	zoxideSessions, err := getZoxideResults()
	if err != nil {
		return nil, err
	}

	var configSessions, configWorktreeSessions []Session

	for _, c := range config {
		if c.Type != nil && *c.Type == "worktreeroot" {
			worktrees, err := getGitWorktrees(c.Path)
			if err != nil {
				return nil, err
			}
			for _, w := range worktrees {
				configWorktreeSessions = append(configWorktreeSessions, Session{
					Name: w, Path: w, Type: *c.Type, Tmuxinator: c.Tmuxinator,
				})
			}
		} else {
			configSessions = append(configSessions, Session{
				Name:       c.Path,
				Path:       c.Path,
				Type:       "",
				Tmuxinator: c.Tmuxinator,
			})
		}
	}

	var sessions []Session
	sessions = append(sessions, tmuxSessions...)
	sessions = append(sessions, configSessions...)
	sessions = append(sessions, configWorktreeSessions...)
	sessions = append(sessions, zoxideSessions...)

	// Replace dots in session names
	for i, s := range sessions {
		sessions[i].Name = strings.ReplaceAll(s.Name, ".", "_")
	}

	return sessions, nil
}

func getTmuxSessions() ([]Session, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error executing tmux command: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	var sessions []Session
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			sessions = append(sessions, Session{Name: trimmed})
		}
	}

	return sessions, nil
}

func getZoxideResults() ([]Session, error) {
	cmd := exec.Command("zoxide", "query", "-l")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error executing zoxide command: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	var results []Session
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			results = append(results, Session{Name: trimmed, Path: trimmed})
		}
	}

	return results, nil
}

func getGitWorktrees(worktreeRoot string) ([]string, error) {
	cmd := exec.Command("git", "-C", worktreeRoot, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error fetching git worktrees: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	var worktrees []string
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			worktrees = append(worktrees, strings.TrimSpace(strings.TrimPrefix(line, "worktree ")))
		}
	}

	return worktrees, nil
}

func switchToTmuxSession(sessionName string) error {
	cmd := exec.Command("tmux", "switch-client", "-t", sessionName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error switching to tmux session: %v", err)
	}
	return nil
}

func createTmuxSession(session Session) error {
	if session.Path == "" {
		return fmt.Errorf("session path is missing, cannot create session")
	}

	if session.Tmuxinator != "" {
		cmd := exec.Command("sh", "-c",
			fmt.Sprintf("cd %s && yes | tmuxinator start -n %s -p %s --no-attach", session.Path, session.Name, session.Tmuxinator),
		)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error starting tmuxinator session: %v", err)
		}
	} else {
		cmd := exec.Command("tmux", "new-session", "-s", session.Name, "-d", "-c", session.Path)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error creating new tmux session: %v", err)
		}
	}

	return nil
}

func listSessionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all available sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := getSessions()
			if err != nil {
				return err
			}

			for _, session := range sessions {
				fmt.Println(session.Name)
			}
			return nil
		},
	}
}

func connectSessionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "connect <session>",
		Short: "Connect to a given session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}

			sessionName := args[0]
			sessions, err := getSessions()
			if err != nil {
				return err
			}

			var selectedSession *Session
			for _, s := range sessions {
				if s.Name == sessionName {
					selectedSession = &s
					break
				}
			}

			if selectedSession == nil {
				return fmt.Errorf("session %s not found", sessionName)
			}

			tmuxSessions, err := getTmuxSessions()
			if err != nil {
				return err
			}

			sessionExists := false
			for _, s := range tmuxSessions {
				if s.Name == selectedSession.Name {
					sessionExists = true
					break
				}
			}

			if !sessionExists {
				if err := createTmuxSession(*selectedSession); err != nil {
					return err
				}
			}

			return switchToTmuxSession(selectedSession.Name)
		},
	}
}

func expandHomePath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		usr, err := user.Current()
		if err != nil {
			return "", err
		}
		path = filepath.Join(usr.HomeDir, path[1:])
	}
	return path, nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "mingle",
		Short: "Tool to improve my workflow by mingling other tools together",
	}

	rootCmd.AddCommand(listSessionsCmd())
	rootCmd.AddCommand(connectSessionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

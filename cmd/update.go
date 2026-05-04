package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/coingecko/coingecko-cli/internal/updater"
	"github.com/spf13/cobra"
)

var fetchLatestFunc = updater.FetchLatest

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Upgrade the CLI to the latest version",
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().String("method", "", "Install method override (homebrew, go, script)")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	method, _ := cmd.Flags().GetString("method")
	if method == "" {
		method = detectInstallMethod()
	} else {
		switch method {
		case "homebrew", "go", "script":
		default:
			return fmt.Errorf("unknown install method %q — must be one of: homebrew, go, script", method)
		}
	}

	warnf("Checking for updates...\n")
	latest, err := fetchLatestFunc()
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}
	if !updater.ValidVersion(latest) {
		return fmt.Errorf("unexpected version format from GitHub: %q", latest)
	}

	currentVer := strings.TrimPrefix(version, "v")
	if latest == currentVer {
		warnf("Already up to date (%s).\n", version)
		return nil
	}
	if updater.VersionGreater(currentVer, latest) {
		warnf("Current version (v%s) is ahead of the latest release (v%s).\n", currentVer, latest)
		return nil
	}

	warnf("Current: v%s  →  Latest: v%s  (install via: %s)\n\n", currentVer, latest, method)

	var confirmed bool
	if err := huh.NewConfirm().
		Title(fmt.Sprintf("Update cg v%s → v%s?", currentVer, latest)).
		Value(&confirmed).
		Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		return err
	}
	if !confirmed {
		return nil
	}

	return runInstallCommand(method)
}

func detectInstallMethod() string {
	exe, err := os.Executable()
	if err != nil {
		return "script"
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "script"
	}
	return classifyInstallPath(exe)
}

// classifyInstallPath returns the install method ("homebrew", "go", or "script")
// for a resolved executable path.
func classifyInstallPath(exe string) string {
	if strings.Contains(exe, "/Cellar/") ||
		strings.Contains(exe, "/homebrew/") ||
		strings.Contains(exe, "/opt/homebrew/") {
		return "homebrew"
	}

	gobin := os.Getenv("GOBIN")
	if gobin == "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			home, _ := os.UserHomeDir()
			gopath = filepath.Join(home, "go")
		}
		gobin = filepath.Join(gopath, "bin")
	}
	if strings.HasPrefix(exe, gobin+string(filepath.Separator)) {
		return "go"
	}

	return "script"
}

func runInstallCommand(method string) error {
	var name string
	var args []string
	switch method {
	case "homebrew":
		name = "brew"
		args = []string{"upgrade", "coingecko/coingecko-cli/cg"}
	case "go":
		name = "go"
		args = []string{"install", "github.com/coingecko/coingecko-cli@latest"}
	default: // "script"
		name = "sh"
		args = []string{"-c", "curl -fsSL https://raw.githubusercontent.com/coingecko/coingecko-cli/main/install.sh | sh"}
	}

	c := exec.Command(name, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

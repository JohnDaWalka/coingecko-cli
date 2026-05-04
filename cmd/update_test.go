package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/coingecko/coingecko-cli/internal/updater"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidVersion(t *testing.T) {
	valid := []string{"1.2.3", "0.0.1", "10.20.30", "0.0.0"}
	for _, v := range valid {
		assert.True(t, updater.ValidVersion(v), "expected valid: %s", v)
	}

	invalid := []string{"v1.2.3", "1.2", "1.2.3.4", "1.2.x", "", "abc", "1.2.", "1.2.3-rc1"}
	for _, v := range invalid {
		assert.False(t, updater.ValidVersion(v), "expected invalid: %s", v)
	}
}

func TestRunUpdate_AlreadyUpToDate(t *testing.T) {
	orig := fetchLatestFunc
	fetchLatestFunc = func() (string, error) { return "1.2.3", nil }
	t.Cleanup(func() { fetchLatestFunc = orig })

	origVersion := version
	version = "1.2.3"
	t.Cleanup(func() { version = origVersion })

	err := updateCmd.RunE(updateCmd, nil)
	assert.NoError(t, err)
}

func TestRunUpdate_CurrentAhead(t *testing.T) {
	orig := fetchLatestFunc
	fetchLatestFunc = func() (string, error) { return "1.0.0", nil }
	t.Cleanup(func() { fetchLatestFunc = orig })

	origVersion := version
	version = "2.0.0"
	t.Cleanup(func() { version = origVersion })

	err := updateCmd.RunE(updateCmd, nil)
	assert.NoError(t, err)
}

func TestRunUpdate_InvalidVersionFromGitHub(t *testing.T) {
	orig := fetchLatestFunc
	fetchLatestFunc = func() (string, error) { return "not-a-version", nil }
	t.Cleanup(func() { fetchLatestFunc = orig })

	origVersion := version
	version = "1.2.3"
	t.Cleanup(func() { version = origVersion })

	err := updateCmd.RunE(updateCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected version format")
}

func TestRunUpdate_InvalidMethod(t *testing.T) {
	require.NoError(t, updateCmd.Flags().Set("method", "invalid"))
	t.Cleanup(func() { _ = updateCmd.Flags().Set("method", "") })

	err := updateCmd.RunE(updateCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown install method")
}

func TestClassifyInstallPath_Homebrew(t *testing.T) {
	cases := []struct {
		path string
		desc string
	}{
		{"/usr/local/Cellar/cg/1.0.0/bin/cg", "Cellar path"},
		{"/home/linuxbrew/.linuxbrew/homebrew/bin/cg", "homebrew path"},
		{"/opt/homebrew/bin/cg", "opt/homebrew path"},
	}
	for _, tc := range cases {
		assert.Equal(t, "homebrew", classifyInstallPath(tc.path), tc.desc)
	}
}

func TestClassifyInstallPath_Go_ExplicitGOBIN(t *testing.T) {
	gobin := t.TempDir()
	t.Setenv("GOBIN", gobin)
	t.Setenv("GOPATH", "")

	exe := filepath.Join(gobin, "cg")
	assert.Equal(t, "go", classifyInstallPath(exe))
}

func TestClassifyInstallPath_Go_DefaultGOPATH(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")

	exe := filepath.Join(home, "go", "bin", "cg")
	assert.Equal(t, "go", classifyInstallPath(exe))
}

func TestClassifyInstallPath_Go_ExplicitGOPATH(t *testing.T) {
	gopath := t.TempDir()
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", gopath)

	exe := filepath.Join(gopath, "bin", "cg")
	assert.Equal(t, "go", classifyInstallPath(exe))
}

func TestClassifyInstallPath_Script(t *testing.T) {
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")

	cases := []string{
		"/usr/local/bin/cg",
		"/home/user/.local/bin/cg",
		"/tmp/cg",
	}
	for _, exe := range cases {
		assert.Equal(t, "script", classifyInstallPath(exe), "path: %s", exe)
	}
}

func TestClassifyInstallPath_GoBinNotParentDir(t *testing.T) {
	gobin := "/home/user/go/bin"
	t.Setenv("GOBIN", gobin)

	// A path that starts with the gobin string but isn't under it (no separator)
	exe := gobin + "extra/cg"
	assert.Equal(t, "script", classifyInstallPath(exe))
}

func TestDetectInstallMethod_ReturnsValidMethod(t *testing.T) {
	method := detectInstallMethod()
	assert.Contains(t, []string{"homebrew", "go", "script"}, method)
}

func TestRunUpdate_FetchError(t *testing.T) {
	orig := fetchLatestFunc
	fetchLatestFunc = func() (string, error) { return "", fmt.Errorf("network timeout") }
	t.Cleanup(func() { fetchLatestFunc = orig })

	origVersion := version
	version = "1.2.3"
	t.Cleanup(func() { version = origVersion })

	err := updateCmd.RunE(updateCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network timeout")
}

package cmd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidVersion(t *testing.T) {
	valid := []string{"1.2.3", "0.0.1", "10.20.30", "0.0.0"}
	for _, v := range valid {
		assert.True(t, validVersion(v), "expected valid: %s", v)
	}

	invalid := []string{"v1.2.3", "1.2", "1.2.3.4", "1.2.x", "", "abc", "1.2."}
	for _, v := range invalid {
		assert.False(t, validVersion(v), "expected invalid: %s", v)
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

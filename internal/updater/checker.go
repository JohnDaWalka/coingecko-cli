package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	githubReleasesURL = "https://api.github.com/repos/coingecko/coingecko-cli/releases/latest"
	cacheTTL          = 24 * time.Hour
)

// Info holds the result of an update check.
type Info struct {
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
}

type cacheEntry struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version"`
}

// Check returns update info, or nil if the check should be skipped or fails.
// Results are cached for 24 hours. Set CG_NO_UPDATE_CHECK=1 to skip.
func Check(currentVersion string) *Info {
	if os.Getenv("CG_NO_UPDATE_CHECK") == "1" || currentVersion == "dev" || currentVersion == "" {
		return nil
	}

	latest := cachedVersion()
	if latest == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		v, err := fetchLatest(ctx)
		if err != nil || v == "" {
			return nil
		}
		saveCache(v)
		latest = v
	}

	return &Info{
		CurrentVersion:  currentVersion,
		LatestVersion:   latest,
		UpdateAvailable: latest != currentVersion,
	}
}

// FetchLatest fetches the latest release tag from GitHub, updates the cache, and returns the tag.
func FetchLatest() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	v, err := fetchLatest(ctx)
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", fmt.Errorf("GitHub returned empty version tag")
	}
	saveCache(v)
	return v, nil
}

func fetchLatest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "coingecko-cli")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return strings.TrimPrefix(release.TagName, "v"), nil
}

func cacheFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "coingecko-cli", "update_check.json"), nil
}

func cachedVersion() string {
	path, err := cacheFilePath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var c cacheEntry
	if err := json.Unmarshal(data, &c); err != nil {
		return ""
	}
	if time.Since(c.CheckedAt) > cacheTTL {
		return ""
	}
	return c.LatestVersion
}

func saveCache(latest string) {
	path, err := cacheFilePath()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	c := cacheEntry{
		CheckedAt:     time.Now(),
		LatestVersion: latest,
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

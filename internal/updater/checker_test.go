package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionGreater(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1.2.3", "1.2.2", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.3", "1.2.4", false},
		{"2.0.0", "1.9.9", true},
		{"1.10.0", "1.9.0", true},  // numeric, not lexicographic
		{"1.9.0", "1.10.0", false}, // lexicographic would be wrong here
		{"0.0.1", "0.0.0", true},
		{"0.1.0", "0.0.9", true},
		{"0.0.0", "0.0.1", false},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, VersionGreater(tc.a, tc.b), "%s > %s", tc.a, tc.b)
	}
}

func releaseServer(t *testing.T, tag string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": tag})
	}))
	origURL := githubReleasesURL
	githubReleasesURL = srv.URL
	t.Cleanup(func() {
		githubReleasesURL = origURL
		srv.Close()
	})
}

func isolateCache(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	orig := userConfigDirFunc
	userConfigDirFunc = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFunc = orig })
}

func TestCheck_SkipsOnDevVersion(t *testing.T) {
	assert.Nil(t, Check("dev"))
}

func TestCheck_SkipsOnEmptyVersion(t *testing.T) {
	assert.Nil(t, Check(""))
}

func TestCheck_SkipsOnEnvVar(t *testing.T) {
	t.Setenv("CG_NO_UPDATE_CHECK", "1")
	assert.Nil(t, Check("1.2.3"))
}

func TestCheck_UpdateAvailable(t *testing.T) {
	isolateCache(t)
	releaseServer(t, "v1.3.0")

	info := Check("1.2.0")
	require.NotNil(t, info)
	assert.True(t, info.UpdateAvailable)
	assert.Equal(t, "1.2.0", info.CurrentVersion)
	assert.Equal(t, "1.3.0", info.LatestVersion)
}

func TestCheck_AlreadyCurrent(t *testing.T) {
	isolateCache(t)
	releaseServer(t, "v1.2.0")

	info := Check("1.2.0")
	require.NotNil(t, info)
	assert.False(t, info.UpdateAvailable)
}

func TestCheck_CurrentAhead(t *testing.T) {
	isolateCache(t)
	releaseServer(t, "v1.2.0")

	info := Check("2.0.0")
	require.NotNil(t, info)
	assert.False(t, info.UpdateAvailable)
}

func TestCheck_UsesCache(t *testing.T) {
	isolateCache(t)
	saveCache("1.9.9")

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v2.0.0"})
	}))
	defer srv.Close()
	origURL := githubReleasesURL
	githubReleasesURL = srv.URL
	defer func() { githubReleasesURL = origURL }()

	info := Check("1.0.0")
	require.NotNil(t, info)
	assert.Equal(t, 0, calls, "should use cache without calling GitHub")
	assert.Equal(t, "1.9.9", info.LatestVersion)
}

func TestFetchLatest_StripsVPrefix(t *testing.T) {
	isolateCache(t)
	releaseServer(t, "v1.5.2")

	v, err := FetchLatest()
	require.NoError(t, err)
	assert.Equal(t, "1.5.2", v)
}

func TestFetchLatest_ServerError(t *testing.T) {
	isolateCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	origURL := githubReleasesURL
	githubReleasesURL = srv.URL
	defer func() { githubReleasesURL = origURL }()

	_, err := FetchLatest()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

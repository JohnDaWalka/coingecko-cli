package cmd

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDryRun_PartialOverrideFallback verifies that when a command defines
// per-mode OASSpecs or OASOperationIDs maps but the requested opKey is NOT
// in those maps, the output falls back to the command-level defaults instead
// of emitting empty strings.
func TestDryRun_PartialOverrideFallback(t *testing.T) {
	// Inject a synthetic command with partial override maps:
	// OASSpecs has "--modeA" but NOT "--modeB".
	// OASOperationIDs has "--modeA" but NOT "--modeB".
	const cmdName = "_test_partial_override"
	commandMeta[cmdName] = commandAnnotation{
		OASSpec:        "default-spec.json",
		OASOperationID: "default-op",
		OASSpecs: map[string]string{
			"--modeA": "modeA-spec.json",
			// "--modeB" deliberately absent
		},
		OASOperationIDs: map[string]string{
			"--modeA": "modeA-op",
			// "--modeB" deliberately absent
		},
		RequiresAuth: true,
	}
	t.Cleanup(func() { delete(commandMeta, cmdName) })

	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("dry-run should not make HTTP calls")
	})
	defer srv.Close()
	withTestClientDemo(t, srv)

	tests := []struct {
		name           string
		opKey          string
		wantSpec       string
		wantOperationID string
	}{
		{
			name:            "opKey present in override maps",
			opKey:           "--modeA",
			wantSpec:        "modeA-spec.json",
			wantOperationID: "modeA-op",
		},
		{
			name:            "opKey missing from override maps falls back to defaults",
			opKey:           "--modeB",
			wantSpec:        "default-spec.json",
			wantOperationID: "default-op",
		},
		{
			name:            "empty opKey uses defaults",
			opKey:           "",
			wantSpec:        "default-spec.json",
			wantOperationID: "default-op",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call printDryRunFull directly by capturing its JSON output via stdout.
			// We use executeCommand with a helper command that calls printDryRunFull.
			// Instead, since printDryRunFull writes to stdout, capture it directly.
			stdout := captureStdout(t, func() {
				cfg, err := loadConfig()
				require.NoError(t, err)
				err = printDryRunFull(cfg, cmdName, tt.opKey, "/test/endpoint", nil, nil, "")
				require.NoError(t, err)
			})

			var out dryRunOutput
			require.NoError(t, json.Unmarshal([]byte(stdout), &out))
			assert.Equal(t, tt.wantSpec, out.OASSpec, "OASSpec")
			assert.Equal(t, tt.wantOperationID, out.OASOperationID, "OASOperationID")
		})
	}
}

package cmd

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/coingecko/coingecko-cli/internal/display"
	"github.com/coingecko/coingecko-cli/internal/updater"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// resolveBuildInfo fills in version/commit/date from the embedded module
// BuildInfo when ldflags weren't injected (e.g. `go install module@vX.Y.Z`).
// goreleaser and `make build` set these via -ldflags and skip this fallback.
func resolveBuildInfo() {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if version == "dev" || version == "" {
		if v := strings.TrimPrefix(bi.Main.Version, "v"); v != "" && v != "(devel)" {
			version = v
		}
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" && commit == "none" {
			commit = s.Value
		}
		if s.Key == "vcs.time" && date == "unknown" {
			date = s.Value
		}
	}
}

var rootCmd = &cobra.Command{
	Use:     "cg",
	Short:   "CoinGecko CLI — cryptocurrency data at your fingertips",
	Long:    "A command-line tool for accessing CoinGecko cryptocurrency market data.",
	Version: version,
	Run: func(cmd *cobra.Command, args []string) {
		display.PrintLogo()
		display.PrintWelcomeBox(version)
		if info := updater.Check(version); info != nil && info.UpdateAvailable {
			display.PrintUpdateReminder(info.CurrentVersion, info.LatestVersion)
		}
	},
}

func init() {
	resolveBuildInfo()
	rootCmd.Version = version
	rootCmd.PersistentFlags().StringP("output", "o", "table", "Output format (table, json)")
}

func Execute() {
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	if err := rootCmd.Execute(); err != nil {
		// Emit structured JSON error to stderr when -o json, otherwise plain text.
		cmd, _, _ := rootCmd.Find(os.Args[1:])
		if cmd != nil && outputJSON(cmd) {
			_ = formatError(cmd, err)
		} else {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		os.Exit(1)
	}
}

func outputJSON(cmd *cobra.Command) bool {
	o, _ := cmd.Flags().GetString("output")
	return o == "json"
}

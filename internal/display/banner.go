package display

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// Brand color: CoinGecko green #4BCC00 → RGB(75, 204, 0)
const (
	brandGreen = "\033[38;2;75;204;0m"
	dimColor   = "\033[2m"
	cyanColor  = "\033[36m"
	yellowBold = "\033[1;33m"
	boxWidth   = 78 // inner width of the welcome box
)

var asciiLogo = []string{
	"  ██████╗ ██████╗ ██╗███╗   ██╗ ██████╗ ███████╗ ██████╗██╗  ██╗ ██████╗ ",
	" ██╔════╝██╔═══██╗██║████╗  ██║██╔════╝ ██╔════╝██╔════╝██║ ██╔╝██╔═══██╗",
	" ██║     ██║   ██║██║██╔██╗ ██║██║  ███╗█████╗  ██║     █████╔╝ ██║   ██║",
	" ██║     ██║   ██║██║██║╚██╗██║██║   ██║██╔══╝  ██║     ██╔═██╗ ██║   ██║",
	" ╚██████╗╚██████╔╝██║██║ ╚████║╚██████╔╝███████╗╚██████╗██║  ██╗╚██████╔╝",
	"  ╚═════╝ ╚═════╝ ╚═╝╚═╝  ╚═══╝ ╚═════╝ ╚══════╝ ╚═════╝╚═╝  ╚═╝ ╚═════╝ ",
}

// PrintLogo prints the full ASCII art CoinGecko logo in brand green to stderr.
func PrintLogo() {
	if !ColorEnabled() {
		for _, line := range asciiLogo {
			_, _ = fmt.Fprintln(os.Stderr, line)
		}
		_, _ = fmt.Fprintln(os.Stderr)
		return
	}
	_, _ = fmt.Fprintln(os.Stderr)
	for _, line := range asciiLogo {
		_, _ = fmt.Fprintf(os.Stderr, "%s%s%s\n", brandGreen, line, colorReset)
	}
	_, _ = fmt.Fprintln(os.Stderr)
}

// PrintWelcomeBox prints a bordered quick-start box to stderr.
// version is the embedded build version (e.g. "v1.2.3") shown in the header.
func PrintWelcomeBox(version string) {
	w := os.Stderr
	top := "+" + strings.Repeat("-", boxWidth) + "+"
	blank := "|" + strings.Repeat(" ", boxWidth) + "|"
	sep := boxRow(w, dimColor+strings.Repeat("-", boxWidth-2)+colorReset, boxWidth-2)

	// "◆ CoinGecko CLI " is 16 runes; version length varies.
	versionVisible := 16 + utf8.RuneCountInString(version)

	_, _ = fmt.Fprintln(w, top)
	_, _ = fmt.Fprintln(w, blank)
	printColoredRow(w, brandGreen+"◆ CoinGecko CLI "+colorReset+version, versionVisible)
	printColoredRow(w, yellowBold+"Official API Command Line Interface"+colorReset, 35)
	_, _ = fmt.Fprintln(w, blank)
	_, _ = fmt.Fprintln(w, sep)
	_, _ = fmt.Fprintln(w, blank)
	printPlainRow(w, "  Quick Start")
	_, _ = fmt.Fprintln(w, blank)
	printCmdRow(w, "cg auth", "# Set up your API key")
	printCmdRow(w, "cg price --ids bitcoin", "# Get BTC price")
	printCmdRow(w, "cg markets --total 100", "# Top 100 by mkt cap")
	printCmdRow(w, "cg search ethereum", "# Search for a coin")
	printCmdRow(w, "cg trending", "# Trending coins")
	printCmdRow(w, "cg history bitcoin --days 30", "# 30-day price history")
	printCmdRow(w, "cg top-gainers-losers", "# Top gainers (paid)")
	printCmdRow(w, "cg watch --ids bitcoin", "# Live prices (paid)")
	printCmdRow(w, "cg tui markets", "# Interactive TUI")
	_, _ = fmt.Fprintln(w, blank)
	_, _ = fmt.Fprintln(w, sep)
	_, _ = fmt.Fprintln(w, blank)
	printColoredRow(w, "  "+dimColor+"Docs: "+colorReset+cyanColor+"https://docs.coingecko.com"+colorReset, 34)
	_, _ = fmt.Fprintln(w, blank)
	_, _ = fmt.Fprintln(w, top)
	_, _ = fmt.Fprintln(w)
}

func printPlainRow(w *os.File, text string) {
	pad := boxWidth - 2 - len(text)
	if pad < 0 {
		pad = 0
	}
	_, _ = fmt.Fprintf(w, "| %s%s |\n", text, strings.Repeat(" ", pad))
}

func printColoredRow(w *os.File, content string, visible int) {
	pad := boxWidth - 2 - visible
	if pad < 0 {
		pad = 0
	}
	if !ColorEnabled() {
		plain := ansiRegex.ReplaceAllString(content, "")
		plainPad := boxWidth - 2 - utf8.RuneCountInString(plain)
		if plainPad < 0 {
			plainPad = 0
		}
		_, _ = fmt.Fprintf(w, "| %s%s |\n", plain, strings.Repeat(" ", plainPad))
		return
	}
	_, _ = fmt.Fprintf(w, "| %s%s |\n", content, strings.Repeat(" ", pad))
}

func printCmdRow(w *os.File, cmd, comment string) {
	// Layout: "| " + "  " + "$" + " " + cmd(30) + " " + comment + pad + " |"
	cmdPad := 30 - len(cmd)
	if cmdPad < 0 {
		cmdPad = 0
	}
	commentPad := 41 - len(comment)
	if commentPad < 0 {
		commentPad = 0
	}
	if ColorEnabled() {
		_, _ = fmt.Fprintf(w, "|   %s$%s %s%s %s%s%s |\n",
			brandGreen, colorReset,
			cmd, strings.Repeat(" ", cmdPad),
			dimColor, comment, colorReset+strings.Repeat(" ", commentPad))
	} else {
		_, _ = fmt.Fprintf(w, "|   $ %s%s %s%s |\n",
			cmd, strings.Repeat(" ", cmdPad),
			comment, strings.Repeat(" ", commentPad))
	}
}

func boxRow(w *os.File, content string, visible int) string {
	pad := boxWidth - 2 - visible
	if pad < 0 {
		pad = 0
	}
	return fmt.Sprintf("| %s%s |", content, strings.Repeat(" ", pad))
}

// PrintUpdateReminder writes a one-liner update notice to stderr.
func PrintUpdateReminder(current, latest string) {
	if ColorEnabled() {
		fmt.Fprintf(os.Stderr, "  %sUpdate available:%s %s → v%s. Run %scg update%s to upgrade.\n\n",
			yellowBold, colorReset, current, latest, yellowBold, colorReset)
	} else {
		fmt.Fprintf(os.Stderr, "  Update available: v%s → v%s. Run `cg update` to upgrade.\n\n", current, latest)
	}
}

// BannerLines is the number of terminal rows FprintBanner occupies
// (leading \n + text + trailing \n\n). Used by watch to position the
// status line without hardcoding a row number.
const BannerLines = 3

// FprintBanner writes a compact one-line CoinGecko banner to w.
// Color is determined by the writer's fd (not the global ColorEnabled check),
// so writing to stdout vs stderr each gets the right color decision.
func FprintBanner(w io.Writer) {
	colored := false
	if os.Getenv("NO_COLOR") == "" {
		if f, ok := w.(*os.File); ok {
			colored = term.IsTerminal(int(f.Fd()))
		}
	}
	if !colored {
		_, _ = fmt.Fprint(w, "\n  CoinGecko CLI  —  Real-time crypto data\n\n")
		return
	}
	_, _ = fmt.Fprintf(w, "\n  %s◆ CoinGecko%s %sCLI  —  Real-time crypto data%s\n\n",
		brandGreen, colorReset, dimColor, colorReset)
}

// PrintBanner prints a compact one-line CoinGecko banner to stderr.
// Writing to stderr keeps stdout clean for piped data.
func PrintBanner() {
	FprintBanner(os.Stderr)
}

package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/coingecko/coingecko-cli/internal/api"
	"github.com/coingecko/coingecko-cli/internal/config"
	"github.com/coingecko/coingecko-cli/internal/display"

	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history [coin-id]",
	Short: "Get historical price data for a coin",
	Long: `Fetch historical data using one of three modes (mutually exclusive):
  --date YYYY-MM-DD     Snapshot on a specific date
  --days N              Price data for the last N days (or OHLC with --ohlc)
  --from/--to           Price data for a date range (or OHLC with --ohlc)

The --to date is inclusive: it covers the full day up to 23:59:59 UTC.
The --interval flag controls data granularity (daily is free for --days; other values require paid plans).
The --ohlc flag switches --days and --from/--to to OHLC output (OHLC --days accepts: 1, 7, 14, 30, 90, 180, 365, max).`,
	Example: `  cg history bitcoin --date 2024-01-01
  cg history ethereum --days 30
  cg history bitcoin --days 7 --ohlc
  cg history bitcoin --days 90 --interval daily
  cg history solana --from 2024-01-01 --to 2024-03-01
  cg history solana --from 2024-01-01 --to 2024-03-01 --ohlc
  cg history solana --from 2024-01-01 --to 2024-03-01 --export prices.csv`,
	Args: cobra.ExactArgs(1),
	RunE: runHistory,
}

func init() {
	historyCmd.Flags().String("date", "", "Snapshot date (YYYY-MM-DD)")
	historyCmd.Flags().String("days", "", "Data for last N days (any integer, or max)")
	historyCmd.Flags().String("from", "", "Range start date (YYYY-MM-DD)")
	historyCmd.Flags().String("to", "", "Range end date (YYYY-MM-DD)")
	historyCmd.Flags().String("vs", "usd", "Target currency")
	historyCmd.Flags().String("interval", "", "Data interval: daily (free for --days), hourly/5m (paid plans)")
	historyCmd.Flags().Bool("ohlc", false, "Output OHLC data instead of price (for --days and --from/--to); --days only accepts 1,7,14,30,90,180,365,max in OHLC mode")
	historyCmd.Flags().String("export", "", "Export to CSV file path")
	rootCmd.AddCommand(historyCmd)
}

func runHistory(cmd *cobra.Command, args []string) error {
	coinID := args[0]
	dateStr, _ := cmd.Flags().GetString("date")
	daysStr, _ := cmd.Flags().GetString("days")
	fromStr, _ := cmd.Flags().GetString("from")
	toStr, _ := cmd.Flags().GetString("to")
	vs, _ := cmd.Flags().GetString("vs")
	interval, _ := cmd.Flags().GetString("interval")
	ohlc, _ := cmd.Flags().GetBool("ohlc")
	exportPath, _ := cmd.Flags().GetString("export")
	jsonOut := outputJSON(cmd)

	if !jsonOut {
		display.PrintBanner()
	}

	modes := 0
	if dateStr != "" {
		modes++
	}
	if daysStr != "" {
		modes++
	}
	if fromStr != "" || toStr != "" {
		modes++
	}
	if modes != 1 {
		return fmt.Errorf("specify exactly one mode: --date, --days, or --from/--to")
	}

	if ohlc && dateStr != "" {
		return fmt.Errorf("--ohlc cannot be used with --date")
	}

	if ohlc && (fromStr != "" || toStr != "") && interval == "" {
		return fmt.Errorf("--ohlc with --from/--to requires --interval (daily or hourly)")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if ohlc && (fromStr != "" || toStr != "") && !cfg.IsPaid() {
		return fmt.Errorf("--ohlc with --from/--to requires a paid plan — upgrade at %s", paidPlanURL)
	}

	if interval != "" && !cfg.IsPaid() {
		// Only --days (market_chart) with interval=daily is free on demo.
		// All other interval values and modes require a paid plan.
		isDaysDailyFree := daysStr != "" && !ohlc && interval == "daily"
		if !isDaysDailyFree {
			return fmt.Errorf("--interval %s requires a paid plan — upgrade at %s", interval, paidPlanURL)
		}
	}

	if daysStr == "max" && !cfg.IsPaid() {
		return fmt.Errorf("--days max requires a paid plan — upgrade at %s", paidPlanURL)
	}

	if ohlc && daysStr != "" && !validEnum("history", "days", daysStr) {
		return fmt.Errorf("invalid --days %q for OHLC — must be one of: 1, 7, 14, 30, 90, 180, 365, max", daysStr)
	}

	if isDryRun(cmd) {
		return historyDryRun(cfg, coinID, dateStr, daysStr, fromStr, toStr, vs, interval, ohlc)
	}

	client := api.NewClient(cfg)
	ctx := cmd.Context()

	switch {
	case dateStr != "":
		return historyDate(ctx, client, coinID, dateStr, vs, jsonOut)
	case daysStr != "":
		if ohlc {
			return historyOHLC(ctx, client, coinID, vs, daysStr, interval, exportPath, jsonOut)
		}
		return historyDays(ctx, client, coinID, vs, daysStr, interval, exportPath, jsonOut)
	default:
		if fromStr == "" || toStr == "" {
			return fmt.Errorf("both --from and --to are required for range mode")
		}
		if ohlc {
			return historyOHLCRange(ctx, client, coinID, vs, fromStr, toStr, interval, exportPath, jsonOut)
		}
		return historyRange(ctx, client, coinID, vs, fromStr, toStr, interval, exportPath, jsonOut)
	}
}

func historyDryRun(cfg *config.Config, coinID, dateStr, daysStr, fromStr, toStr, vs, interval string, ohlc bool) error {
	switch {
	case dateStr != "":
		t, err := parseDate("--date", dateStr)
		if err != nil {
			return err
		}
		return printDryRunWithOp(cfg, "history", "--date", "/coins/"+coinID+"/history", map[string]string{
			"date": t.Format("02-01-2006"), "localization": "false",
		}, nil)
	case daysStr != "":
		params := map[string]string{"vs_currency": vs, "days": daysStr}
		if interval != "" {
			params["interval"] = interval
		}
		opKey := "--days"
		endpoint := "/coins/" + coinID + "/market_chart"
		if ohlc {
			opKey = "--days --ohlc"
			endpoint = "/coins/" + coinID + "/ohlc"
		}
		return printDryRunWithOp(cfg, "history", opKey, endpoint, params, nil)
	default:
		fromUnix, toUnix, err := parseRange(fromStr, toStr)
		if err != nil {
			return err
		}
		params := map[string]string{
			"vs_currency": vs,
			"from":        fmt.Sprintf("%d", fromUnix),
			"to":          fmt.Sprintf("%d", toUnix),
		}
		if interval != "" {
			params["interval"] = interval
		}
		opKey := "--from/--to"
		endpoint := "/coins/" + coinID + "/market_chart/range"
		if ohlc {
			opKey = "--from/--to --ohlc"
			endpoint = "/coins/" + coinID + "/ohlc/range"
		}
		return printDryRunWithOp(cfg, "history", opKey, endpoint, params, nil)
	}
}

const dateLayout = "2006-01-02"

func parseDate(name, value string) (time.Time, error) {
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s date, use YYYY-MM-DD: %w", name, err)
	}
	return t, nil
}

func endOfDayUnix(t time.Time) int64 {
	return t.Add(23*time.Hour + 59*time.Minute + 59*time.Second).UTC().Unix()
}

func parseRange(fromStr, toStr string) (fromUnix, toUnix int64, err error) {
	fromTime, err := parseDate("--from", fromStr)
	if err != nil {
		return 0, 0, err
	}
	toTime, err := parseDate("--to", toStr)
	if err != nil {
		return 0, 0, err
	}
	return fromTime.UTC().Unix(), endOfDayUnix(toTime), nil
}

func historyDate(ctx context.Context, client *api.Client, coinID, dateStr, vs string, jsonOut bool) error {
	t, err := parseDate("--date", dateStr)
	if err != nil {
		return err
	}
	apiDate := t.Format("02-01-2006") // CoinGecko uses DD-MM-YYYY

	data, err := client.CoinHistory(ctx, coinID, apiDate)
	if err != nil {
		return err
	}

	if data.MarketData == nil {
		return fmt.Errorf("no market data available for %s on %s", coinID, dateStr)
	}

	if jsonOut {
		return printJSONRaw(data)
	}

	headers := []string{"Metric", "Value"}
	rows := [][]string{
		{"Coin", fmt.Sprintf("%s (%s)", display.SanitizeCell(data.Name), display.FormatSymbol(data.Symbol))},
		{"Date", dateStr},
		{"Price", display.FormatPrice(data.MarketData.CurrentPrice[vs], vs)},
		{"Market Cap", display.FormatLargeNumber(data.MarketData.MarketCap[vs], vs)},
		{"Volume", display.FormatLargeNumber(data.MarketData.TotalVolume[vs], vs)},
	}
	display.PrintTable(headers, rows)
	return nil
}

func historyDays(ctx context.Context, client *api.Client, coinID, vs, days, interval, exportPath string, jsonOut bool) error {
	data, err := client.CoinMarketChart(ctx, coinID, vs, days, interval)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSONRaw(data)
	}

	return renderPriceTable(data.Prices, vs, exportPath)
}

func historyRange(ctx context.Context, client *api.Client, coinID, vs, fromStr, toStr, interval, exportPath string, jsonOut bool) error {
	fromUnix, toUnix, err := parseRange(fromStr, toStr)
	if err != nil {
		return err
	}

	data, err := client.CoinMarketChartRange(ctx, coinID, vs, fromUnix, toUnix, interval)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSONRaw(data)
	}

	return renderPriceTable(data.Prices, vs, exportPath)
}

func historyOHLC(ctx context.Context, client *api.Client, coinID, vs, days, interval, exportPath string, jsonOut bool) error {
	data, err := client.CoinOHLC(ctx, coinID, vs, days, interval)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSONRaw(data)
	}

	return renderOHLCTable(data, vs, exportPath)
}

func historyOHLCRange(ctx context.Context, client *api.Client, coinID, vs, fromStr, toStr, interval, exportPath string, jsonOut bool) error {
	fromUnix, toUnix, err := parseRange(fromStr, toStr)
	if err != nil {
		return err
	}

	data, err := client.CoinOHLCRange(ctx, coinID, vs, fromUnix, toUnix, interval)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSONRaw(data)
	}

	return renderOHLCTable(data, vs, exportPath)
}

func renderPriceTable(prices [][]float64, vs, exportPath string) error {
	headers := []string{"Date", "Price"}
	rows := make([][]string, 0, len(prices))
	var csvRows [][]string
	if exportPath != "" {
		csvRows = make([][]string, 0, len(prices))
	}
	for _, p := range prices {
		if len(p) < 2 {
			continue
		}
		ts := time.UnixMilli(int64(p[0]))
		rows = append(rows, []string{
			ts.UTC().Format("2006-01-02 15:04"),
			display.FormatPrice(p[1], vs),
		})
		if exportPath != "" {
			csvRows = append(csvRows, []string{
				ts.UTC().Format(time.RFC3339),
				fmt.Sprintf("%.8f", p[1]),
			})
		}
	}

	display.PrintTable(headers, rows)

	if exportPath != "" {
		if err := exportCSV(exportPath, headers, csvRows); err != nil {
			return err
		}
	}
	return nil
}

func renderOHLCTable(data api.OHLCData, vs, exportPath string) error {
	headers := []string{"Date", "Open", "High", "Low", "Close"}
	rows := make([][]string, 0, len(data))
	var csvRows [][]string
	if exportPath != "" {
		csvRows = make([][]string, 0, len(data))
	}
	for _, d := range data {
		if len(d) < 5 {
			continue
		}
		ts := time.UnixMilli(int64(d[0]))
		rows = append(rows, []string{
			ts.UTC().Format("2006-01-02 15:04"),
			display.FormatPrice(d[1], vs),
			display.FormatPrice(d[2], vs),
			display.FormatPrice(d[3], vs),
			display.FormatPrice(d[4], vs),
		})
		if exportPath != "" {
			csvRows = append(csvRows, []string{
				ts.UTC().Format(time.RFC3339),
				fmt.Sprintf("%.8f", d[1]),
				fmt.Sprintf("%.8f", d[2]),
				fmt.Sprintf("%.8f", d[3]),
				fmt.Sprintf("%.8f", d[4]),
			})
		}
	}

	display.PrintTable(headers, rows)

	if exportPath != "" {
		if err := exportCSV(exportPath, headers, csvRows); err != nil {
			return err
		}
	}
	return nil
}

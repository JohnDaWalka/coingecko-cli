package cmd

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/coingecko/coingecko-cli/internal/display"

	"github.com/spf13/cobra"
)

var contractCmd = &cobra.Command{
	Use:   "contract",
	Short: "Get token price by contract address",
	Long: `Fetch token price by contract address. Uses CoinGecko's aggregated price by
default, or DEX price from GeckoTerminal with --onchain.

Find valid --platform IDs:    https://docs.coingecko.com/reference/asset-platforms-list
Find valid --network IDs:     https://docs.coingecko.com/reference/networks-list

Note: --platform (e.g. "ethereum") and --network (e.g. "eth") are different
identifiers from different API specs — they are not interchangeable.`,
	Example: `  cg contract --address 0x1f98... --platform ethereum
  cg contract --address 0x1f98... --platform ethereum --vs eur
  cg contract --address 0x1f98... --network eth --onchain
  cg contract --address 0x1f98... --network eth --onchain --vs eur`,
	RunE: runContract,
}

func init() {
	contractCmd.Flags().String("address", "", "Contract address (required)")
	contractCmd.Flags().String("platform", "", "Platform ID for aggregated mode (e.g. ethereum). See https://docs.coingecko.com/reference/asset-platforms-list")
	contractCmd.Flags().String("network", "", "Network ID for onchain mode (e.g. eth). See https://docs.coingecko.com/reference/networks-list")
	contractCmd.Flags().Bool("onchain", false, "Use DEX price from GeckoTerminal")
	contractCmd.Flags().String("vs", "usd", "Target currency")
	contractCmd.Flags().String("export", "", "Export to CSV file path")
	rootCmd.AddCommand(contractCmd)
}

func runContract(cmd *cobra.Command, args []string) error {
	address, _ := cmd.Flags().GetString("address")
	platform, _ := cmd.Flags().GetString("platform")
	network, _ := cmd.Flags().GetString("network")
	onchain, _ := cmd.Flags().GetBool("onchain")
	vs, _ := cmd.Flags().GetString("vs")
	vs = strings.ToLower(vs)
	exportPath, _ := cmd.Flags().GetString("export")
	jsonOut := outputJSON(cmd)

	if !jsonOut {
		display.PrintBanner()
	}

	// Validation — fail fast before config load.
	if address == "" {
		return fmt.Errorf("--address is required")
	}
	if onchain && platform != "" {
		return fmt.Errorf("--platform and --onchain are mutually exclusive")
	}
	if network != "" && !onchain {
		return fmt.Errorf("--network requires --onchain flag; did you mean --platform?")
	}
	if onchain && network == "" {
		return fmt.Errorf("--network is required with --onchain; find valid network IDs at https://docs.coingecko.com/reference/networks-list")
	}
	if !onchain && platform == "" {
		return fmt.Errorf("--platform is required (or use --onchain with --network); find valid platform IDs at https://docs.coingecko.com/reference/asset-platforms-list")
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Dry-run path.
	if isDryRun(cmd) {
		if onchain {
			params := map[string]string{
				"include_market_cap":        "true",
				"include_24hr_vol":          "true",
				"include_24hr_price_change": "true",
			}
			note := ""
			if vs != "usd" {
				note = fmt.Sprintf("Additional request: GET /exchange_rates (currency conversion from USD to %s)", vs)
			}
			return printDryRunFull(cfg, "contract", "--onchain",
				fmt.Sprintf("/onchain/simple/networks/%s/token_price/%s", url.PathEscape(network), address),
				params, nil, note)
		}
		params := map[string]string{
			"contract_addresses":  address,
			"vs_currencies":       vs,
			"include_market_cap":  "true",
			"include_24hr_vol":    "true",
			"include_24hr_change": "true",
		}
		return printDryRunWithOp(cfg, "contract", "default",
			"/simple/token_price/"+url.PathEscape(platform), params, nil)
	}

	client := newAPIClient(cfg)
	ctx := cmd.Context()

	// Shared output variables.
	var price, marketCap, volume, change float64
	var displayAddr string

	if onchain {
		// Onchain path.
		resp, err := client.OnchainSimpleTokenPrice(ctx, network, []string{address})
		if err != nil {
			return err
		}

		priceStr, ok := resp.Data.Attributes.TokenPrices[address]
		if !ok {
			return fmt.Errorf("no data returned for address %s", address)
		}
		displayAddr = address

		price, err = strconv.ParseFloat(priceStr, 64)
		if err != nil {
			return fmt.Errorf("parsing price: %w", err)
		}

		if mcStr, ok := resp.Data.Attributes.MarketCapUSD[address]; ok {
			marketCap, _ = strconv.ParseFloat(mcStr, 64)
		}
		if volStr, ok := resp.Data.Attributes.H24VolumeUSD[address]; ok {
			volume, _ = strconv.ParseFloat(volStr, 64)
		}
		if chgStr, ok := resp.Data.Attributes.H24PriceChangePct[address]; ok {
			change, _ = strconv.ParseFloat(chgStr, 64)
		}

		// Currency conversion for non-USD.
		if vs != "usd" {
			rates, err := client.ExchangeRates(ctx)
			if err != nil {
				return fmt.Errorf("fetching exchange rates: %w", err)
			}
			usdRate, usdOK := rates.Rates["usd"]
			targetRate, targetOK := rates.Rates[vs]
			if !usdOK || !targetOK {
				return fmt.Errorf("unsupported currency %q", vs)
			}
			factor := targetRate.Value / usdRate.Value
			price *= factor
			marketCap *= factor
			volume *= factor
			// 24h change % stays the same
		}
	} else {
		// Aggregated path.
		resp, err := client.SimpleTokenPrice(ctx, platform, []string{address}, vs)
		if err != nil {
			return err
		}

		data, ok := resp[address]
		if !ok {
			return fmt.Errorf("no data returned for address %s", address)
		}
		displayAddr = address

		price = data[vs]
		marketCap = data[vs+"_market_cap"]
		volume = data[vs+"_24h_vol"]
		change = data[vs+"_24h_change"]
	}

	if jsonOut {
		normalized := map[string]interface{}{
			displayAddr: map[string]interface{}{
				"price":      price,
				"market_cap": marketCap,
				"volume_24h": volume,
				"change_24h": change,
			},
		}
		return printJSONRaw(normalized)
	}

	// Table output.
	headers := []string{"Address", "Price", "Market Cap", "24h Volume", "24h Change"}
	rows := [][]string{
		{
			display.SanitizeCell(displayAddr),
			display.FormatPrice(price, vs),
			display.FormatLargeNumber(marketCap, vs),
			display.FormatLargeNumber(volume, vs),
			display.ColorPercent(change),
		},
	}
	display.PrintTable(headers, rows)

	if exportPath != "" {
		csvRows := [][]string{
			{
				display.SanitizeCell(displayAddr),
				fmt.Sprintf("%.8f", price),
				fmt.Sprintf("%.2f", marketCap),
				fmt.Sprintf("%.2f", volume),
				fmt.Sprintf("%.2f", change),
			},
		}
		if err := exportCSV(exportPath, headers, csvRows); err != nil {
			return err
		}
	}

	return nil
}

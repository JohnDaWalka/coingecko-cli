package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/coingecko/coingecko-cli/internal/api"
	"github.com/coingecko/coingecko-cli/internal/display"

	"github.com/spf13/cobra"
)

var contractCmd = &cobra.Command{
	Use:   "contract",
	Short: "Get token price by contract address",
	Long: `Fetch token price by contract address. Uses CoinGecko's aggregated price by
default, or DEX price from GeckoTerminal with --onchain.

When --platform and --network are omitted, smart routing automatically detects
the token's network via pool search, then tries the aggregated CoinGecko price
first and falls back to onchain DEX price if unavailable.

Find valid --platform IDs:    https://docs.coingecko.com/reference/asset-platforms-list
Find valid --network IDs:     https://docs.coingecko.com/reference/networks-list

Note: --platform (e.g. "ethereum") and --network (e.g. "eth") are different
identifiers from different API specs — they are not interchangeable.`,
	Example: `  cg contract --address 0x1f98...                        # smart routing
  cg contract --address 0x1f98... --platform ethereum    # explicit CG aggregated
  cg contract --address 0x1f98... --platform ethereum --vs eur
  cg contract --address 0x1f98... --onchain              # smart routing, onchain only
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

type resolvedAddress struct {
	network  string // onchain network ID (e.g. "eth")
	platform string // CG asset platform ID (e.g. "ethereum"), may be empty
}

// resolveAddress searches onchain pools to find which network a contract address
// lives on, then maps the network to its CoinGecko asset platform ID.
// The two API calls run in parallel since they are independent.
func resolveAddress(ctx context.Context, client *api.Client, address string) (*resolvedAddress, error) {
	var (
		pools    *api.OnchainSearchPoolsResponse
		networks *api.OnchainNetworksResponse
		poolsErr, networksErr error
	)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		pools, poolsErr = client.OnchainSearchPools(ctx, address)
	}()
	go func() {
		defer wg.Done()
		networks, networksErr = client.OnchainNetworks(ctx)
	}()
	wg.Wait()

	if poolsErr != nil {
		return nil, fmt.Errorf("searching for address: %w", poolsErr)
	}
	if len(pools.Data) == 0 {
		return nil, fmt.Errorf("no pools found for address %s — specify --platform or --network manually", address)
	}

	// Token relationship IDs follow the format "{network}_{token_address}".
	// Find the token matching the queried address and extract the network prefix.
	addrLower := strings.ToLower(address)
	pool := pools.Data[0]
	var networkID string
	for _, tokenID := range []string{
		pool.Relationships.BaseToken.Data.ID,
		pool.Relationships.QuoteToken.Data.ID,
	} {
		lower := strings.ToLower(tokenID)
		if strings.HasSuffix(lower, "_"+addrLower) {
			networkID = tokenID[:len(tokenID)-len(address)-1]
			break
		}
	}
	if networkID == "" {
		return nil, fmt.Errorf("could not determine network for address %s", address)
	}

	if networksErr != nil {
		return nil, fmt.Errorf("fetching network mappings: %w", networksErr)
	}

	var platformID string
	for _, n := range networks.Data {
		if n.ID == networkID {
			platformID = n.Attributes.CoingeckoAssetPlatformID
			break
		}
	}

	return &resolvedAddress{network: networkID, platform: platformID}, nil
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

	if address == "" {
		return fmt.Errorf("--address is required")
	}
	if onchain && platform != "" {
		return fmt.Errorf("--platform and --onchain are mutually exclusive")
	}
	if network != "" && !onchain {
		return fmt.Errorf("--network requires --onchain flag; did you mean --platform?")
	}

	needsResolve := (!onchain && platform == "") || (onchain && network == "")

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if isDryRun(cmd) {
		if needsResolve {
			note := "Smart routing: searches pools to determine network, then fetches /onchain/networks for platform mapping. " +
				"After resolution, the aggregated CG endpoint is tried first; if no data, falls back to onchain."
			return printDryRunFull(cfg, "contract", "resolve",
				"/onchain/search/pools",
				map[string]string{"query": address}, nil, note)
		}
		if onchain {
			params := map[string]string{
				"include_market_cap":           "true",
				"include_24hr_vol":             "true",
				"include_24hr_price_change":    "true",
				"mcap_fdv_fallback":            "true",
				"include_total_reserve_in_usd": "true",
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

	if needsResolve {
		resolved, err := resolveAddress(ctx, client, address)
		if err != nil {
			return err
		}

		if resolved.platform != "" {
			warnf("Resolved address to network=%s, platform=%s\n", resolved.network, resolved.platform)
		} else {
			warnf("Resolved address to network=%s (no CG platform mapping)\n", resolved.network)
		}

		if onchain {
			network = resolved.network
		} else {
			platform = resolved.platform
			network = resolved.network
		}
	}

	var price, marketCap, volume, change, reserve float64

	if !onchain && platform != "" {
		resp, err := client.SimpleTokenPrice(ctx, platform, []string{address}, vs)
		if err != nil {
			return err
		}
		if data, ok := resp[address]; ok {
			price = data[vs]
			marketCap = data[vs+"_market_cap"]
			volume = data[vs+"_24h_vol"]
			change = data[vs+"_24h_change"]
		} else if network != "" {
			warnf("No aggregated price found; falling back to onchain (network=%s)\n", network)
			onchain = true
		} else {
			return fmt.Errorf("no data returned for address %s", address)
		}
	}

	if !onchain && platform == "" && network != "" {
		warnf("No CG platform mapping; using onchain (network=%s)\n", network)
		onchain = true
	}

	if onchain {
		resp, err := client.OnchainSimpleTokenPrice(ctx, network, []string{address})
		if err != nil {
			return err
		}

		attrs := resp.Data.Attributes
		priceStr, ok := attrs.TokenPrices[address]
		if !ok {
			return fmt.Errorf("no data returned for address %s", address)
		}

		price, err = strconv.ParseFloat(priceStr, 64)
		if err != nil {
			return fmt.Errorf("parsing price: %w", err)
		}

		if mcStr, ok := attrs.MarketCapUSD[address]; ok {
			marketCap, _ = strconv.ParseFloat(mcStr, 64)
		}
		if volStr, ok := attrs.H24VolumeUSD[address]; ok {
			volume, _ = strconv.ParseFloat(volStr, 64)
		}
		if chgStr, ok := attrs.H24PriceChangePct[address]; ok {
			change, _ = strconv.ParseFloat(chgStr, 64)
		}
		if resStr, ok := attrs.TotalReserveInUSD[address]; ok {
			reserve, _ = strconv.ParseFloat(resStr, 64)
		}

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
			reserve *= factor
			// 24h change % stays the same
		}
	}

	if jsonOut {
		data := map[string]interface{}{
			"price":      price,
			"market_cap": marketCap,
			"volume_24h": volume,
			"change_24h": change,
		}
		if onchain {
			data["total_reserve"] = reserve
		}
		return printJSONRaw(map[string]interface{}{address: data})
	}

	headers := []string{"Address", "Price", "Market Cap", "24h Volume", "24h Change"}
	row := []string{
		display.SanitizeCell(address),
		display.FormatPrice(price, vs),
		display.FormatLargeNumber(marketCap, vs),
		display.FormatLargeNumber(volume, vs),
		display.ColorPercent(change),
	}
	if onchain {
		headers = append(headers, "Reserve")
		row = append(row, display.FormatLargeNumber(reserve, vs))
	}
	display.PrintTable(headers, [][]string{row})

	if exportPath != "" {
		csvRow := []string{
			display.SanitizeCell(address),
			fmt.Sprintf("%.8f", price),
			fmt.Sprintf("%.2f", marketCap),
			fmt.Sprintf("%.2f", volume),
			fmt.Sprintf("%.2f", change),
		}
		if onchain {
			csvRow = append(csvRow, fmt.Sprintf("%.2f", reserve))
		}
		if err := exportCSV(exportPath, headers, [][]string{csvRow}); err != nil {
			return err
		}
	}

	return nil
}

package cmd

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContract_MissingAddress(t *testing.T) {
	_, _, err := executeCommand(t, "contract", "--platform", "ethereum", "-o", "json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--address is required")
}

func TestContract_MissingPlatform(t *testing.T) {
	_, _, err := executeCommand(t, "contract", "--address", "0xabc", "-o", "json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--platform is required")
	assert.Contains(t, err.Error(), "asset-platforms-list")
}

func TestContract_MutualExclusion(t *testing.T) {
	_, _, err := executeCommand(t, "contract", "--address", "0xabc", "--platform", "ethereum", "--onchain", "-o", "json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--platform and --onchain are mutually exclusive")
}

func TestContract_OnchainMissingNetwork(t *testing.T) {
	_, _, err := executeCommand(t, "contract", "--address", "0xabc", "--onchain", "-o", "json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--network is required with --onchain")
	assert.Contains(t, err.Error(), "networks-list")
}

func TestContract_NetworkWithoutOnchain(t *testing.T) {
	_, _, err := executeCommand(t, "contract", "--address", "0xabc", "--network", "eth", "-o", "json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--network requires --onchain flag")
}

func TestContract_DryRun_Aggregated(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not make HTTP call in dry-run mode")
	})
	defer srv.Close()
	withTestClientDemo(t, srv)

	stdout, _, err := executeCommand(t, "contract", "--address", "0xabc", "--platform", "ethereum", "--dry-run", "-o", "json")
	require.NoError(t, err)

	var out dryRunOutput
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	assert.Equal(t, "GET", out.Method)
	assert.Contains(t, out.URL, "/simple/token_price/ethereum")
	assert.Equal(t, "0xabc", out.Params["contract_addresses"])
	assert.Equal(t, "usd", out.Params["vs_currencies"])
	assert.Equal(t, "true", out.Params["include_market_cap"])
	assert.Equal(t, "true", out.Params["include_24hr_vol"])
	assert.Equal(t, "true", out.Params["include_24hr_change"])
	assert.Equal(t, "simple-token-price", out.OASOperationID)
}

func TestContract_DryRun_Onchain(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not make HTTP call in dry-run mode")
	})
	defer srv.Close()
	withTestClientPaid(t, srv)

	stdout, _, err := executeCommand(t, "contract", "--address", "0xabc", "--network", "eth", "--onchain", "--dry-run", "-o", "json")
	require.NoError(t, err)

	var out dryRunOutput
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	assert.Equal(t, "GET", out.Method)
	assert.Contains(t, out.URL, "/onchain/simple/networks/eth/token_price/0xabc")
	assert.Equal(t, "true", out.Params["include_market_cap"])
	assert.Equal(t, "true", out.Params["include_24hr_vol"])
	assert.Equal(t, "true", out.Params["include_24hr_price_change"])
	assert.Equal(t, "onchain-simple-price", out.OASOperationID)
	assert.Empty(t, out.Note)
}

func TestContract_DryRun_Onchain_NonUSD(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not make HTTP call in dry-run mode")
	})
	defer srv.Close()
	withTestClientPaid(t, srv)

	stdout, _, err := executeCommand(t, "contract", "--address", "0xabc", "--network", "eth", "--onchain", "--vs", "eur", "--dry-run", "-o", "json")
	require.NoError(t, err)

	var out dryRunOutput
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	assert.Contains(t, out.Note, "exchange_rates")
	assert.Contains(t, out.Note, "eur")
}

func TestContract_Aggregated_JSONOutput(t *testing.T) {
	addr := "0xdac17f958d2ee523a2206206994597c13d831ec7"
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/simple/token_price/ethereum", r.URL.Path)
		assert.Equal(t, addr, r.URL.Query().Get("contract_addresses"))
		assert.Equal(t, "usd", r.URL.Query().Get("vs_currencies"))

		resp := map[string]map[string]float64{
			addr: {
				"usd":            1.001,
				"usd_market_cap": 50000000000,
				"usd_24h_vol":    30000000000,
				"usd_24h_change": 0.05,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()
	withTestClientDemo(t, srv)

	stdout, _, err := executeCommand(t, "contract", "--address", addr, "--platform", "ethereum", "-o", "json")
	require.NoError(t, err)

	var result map[string]map[string]float64
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	assert.Equal(t, 1.001, result[addr]["price"])
	assert.Equal(t, float64(50000000000), result[addr]["market_cap"])
	assert.Equal(t, float64(30000000000), result[addr]["volume_24h"])
	assert.Equal(t, 0.05, result[addr]["change_24h"])
}

func TestContract_Aggregated_TableOutput(t *testing.T) {
	addr := "0xdac17f958d2ee523a2206206994597c13d831ec7"
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]map[string]float64{
			addr: {
				"usd":            1.001,
				"usd_market_cap": 50000000000,
				"usd_24h_vol":    30000000000,
				"usd_24h_change": 2.5,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()
	withTestClientDemo(t, srv)

	stdout, _, err := executeCommand(t, "contract", "--address", addr, "--platform", "ethereum")
	require.NoError(t, err)
	assert.Contains(t, stdout, addr)
	assert.Contains(t, stdout, "Price")
	assert.Contains(t, stdout, "Market Cap")
	assert.Contains(t, stdout, "24h Volume")
	assert.Contains(t, stdout, "24h Change")
}

func TestContract_Aggregated_NoData(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer srv.Close()
	withTestClientDemo(t, srv)

	_, _, err := executeCommand(t, "contract", "--address", "0xnonexistent", "--platform", "ethereum", "-o", "json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data returned for address")
}

func TestContract_Onchain_JSONOutput(t *testing.T) {
	addr := "0xc02aaa39b223fe8d0a0e5c4f27ead9083c756cc2"
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.HasPrefix(r.URL.Path, "/onchain/simple/networks/eth/token_price/"))

		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id":   "123",
				"type": "simple_token_price",
				"attributes": map[string]interface{}{
					"token_prices":                map[string]string{addr: "2289.33"},
					"market_cap_usd":              map[string]string{addr: "6692452895.78"},
					"h24_volume_usd":              map[string]string{addr: "965988358.73"},
					"h24_price_change_percentage": map[string]string{addr: "3.387"},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()
	withTestClientPaid(t, srv)

	stdout, _, err := executeCommand(t, "contract", "--address", addr, "--network", "eth", "--onchain", "-o", "json")
	require.NoError(t, err)

	var result map[string]map[string]float64
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	assert.InDelta(t, 2289.33, result[addr]["price"], 0.01)
	assert.InDelta(t, 6692452895.78, result[addr]["market_cap"], 0.01)
	assert.InDelta(t, 965988358.73, result[addr]["volume_24h"], 0.01)
	assert.InDelta(t, 3.387, result[addr]["change_24h"], 0.001)
}

func TestContract_Onchain_NonUSD_Conversion(t *testing.T) {
	addr := "0xaddr"
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/onchain/simple/networks/"):
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"id":   "123",
					"type": "simple_token_price",
					"attributes": map[string]interface{}{
						"token_prices":                map[string]string{addr: "2000"},
						"market_cap_usd":              map[string]string{addr: "1000000"},
						"h24_volume_usd":              map[string]string{addr: "500000"},
						"h24_price_change_percentage": map[string]string{addr: "5.0"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case r.URL.Path == "/exchange_rates":
			resp := map[string]interface{}{
				"rates": map[string]interface{}{
					"usd": map[string]interface{}{"name": "US Dollar", "unit": "$", "value": 67187.0, "type": "fiat"},
					"eur": map[string]interface{}{"name": "Euro", "unit": "\u20ac", "value": 62345.0, "type": "fiat"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	})
	defer srv.Close()
	withTestClientPaid(t, srv)

	stdout, _, err := executeCommand(t, "contract", "--address", addr, "--network", "eth", "--onchain", "--vs", "eur", "-o", "json")
	require.NoError(t, err)

	var result map[string]map[string]float64
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))

	// factor = 62345 / 67187 ≈ 0.9280
	factor := 62345.0 / 67187.0
	assert.InDelta(t, 2000*factor, result[addr]["price"], 0.01)
	assert.InDelta(t, 1000000*factor, result[addr]["market_cap"], 0.01)
	assert.InDelta(t, 500000*factor, result[addr]["volume_24h"], 0.01)
	// 24h change should be unchanged
	assert.InDelta(t, 5.0, result[addr]["change_24h"], 0.001)
}

func TestContract_Onchain_UnsupportedCurrency(t *testing.T) {
	addr := "0xaddr"
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/onchain/simple/networks/"):
			resp := map[string]interface{}{
				"data": map[string]interface{}{
					"id":   "123",
					"type": "simple_token_price",
					"attributes": map[string]interface{}{
						"token_prices":                map[string]string{addr: "2000"},
						"market_cap_usd":              map[string]string{addr: "1000000"},
						"h24_volume_usd":              map[string]string{addr: "500000"},
						"h24_price_change_percentage": map[string]string{addr: "5.0"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case r.URL.Path == "/exchange_rates":
			resp := map[string]interface{}{
				"rates": map[string]interface{}{
					"usd": map[string]interface{}{"name": "US Dollar", "unit": "$", "value": 67187.0, "type": "fiat"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	})
	defer srv.Close()
	withTestClientPaid(t, srv)

	_, _, err := executeCommand(t, "contract", "--address", addr, "--network", "eth", "--onchain", "--vs", "xyz", "-o", "json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported currency")
}

func TestContract_Onchain_RequiresPaid(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach the server with a demo client")
	})
	defer srv.Close()
	withTestClientDemo(t, srv)

	_, _, err := executeCommand(t, "contract", "--address", "0xabc", "--network", "eth", "--onchain", "-o", "json")
	require.Error(t, err)
}

func TestContract_Export_CSV(t *testing.T) {
	addr := "0xdac17f958d2ee523a2206206994597c13d831ec7"
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]map[string]float64{
			addr: {
				"usd":            1.001,
				"usd_market_cap": 50000000000,
				"usd_24h_vol":    30000000000,
				"usd_24h_change": 0.05,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()
	withTestClientDemo(t, srv)

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "contract.csv")

	_, _, err := executeCommand(t, "contract", "--address", addr, "--platform", "ethereum", "--export", csvPath)
	require.NoError(t, err)

	data, err := os.ReadFile(csvPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "Address")
	assert.Contains(t, content, "Price")
	assert.Contains(t, content, "Market Cap")
	assert.Contains(t, content, addr)
}

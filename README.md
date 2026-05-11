# cg — CoinGecko CLI

A fast, full-featured terminal interface for the [CoinGecko API](https://docs.coingecko.com), built in Go. Includes an interactive TUI with 7-day price charts, CSV export, 50+ currency symbols, 500+ categories, and more than 10 years of historical data. 

> [!NOTE]
> CoinGecko CLI is currently in Beta.
> We're constantly improving, and your feedback is crucial. Please share your feedback via this [form](https://forms.gle/VgpVbwsSJLgE7D8Q7), or submit a PR.

<img width="584" height="705" alt="Screenshot 2026-03-07 at 4 31 27 PM" src="https://github.com/user-attachments/assets/7973c959-233b-4cb4-8f9e-37f3954c67a3" />


## Features at a Glance
- **🎮 Interactive TUI** — Full-screen terminal dashboard with live navigation and 7-day braille price charts
- **⚡ Real-Time Prices** — Current prices by CoinGecko ID or ticker symbol, with 50+ currency symbols
- **📅 Deep Historical Data** — Snapshots, OHLC, and custom date ranges with CSV export
- **🏷️ Category Filtering** — Filter by 500+ categories including AI, Layer-2, Tokenized Stocks, Gold, and Silver
- **📊 Unlimited Markets** — Fetch 1,000+ coins with automatic pagination
- **🔥 Trending & Top Movers** — Real-time trending coins, NFTs, and categories
- **📥 CSV Export** — Export any market or history query for analysis in Excel or Python
- **📡 Live WebSocket Streaming** — Real-time price updates via `cg watch` with NDJSON output for piping
- **⌨️ JSON Output** — Machine-readable `-o json` for scripting and pipelines
- **🤖 Agent/LLM Friendly** — `--dry-run` mode and `cg commands` for tool integration

<img width="701" height="412" alt="Screenshot 2026-03-07 at 4 06 14 PM" src="https://github.com/user-attachments/assets/a62b26d3-da95-4040-b421-371409202e82" />
<img width="774" height="562" alt="Screenshot 2026-03-07 at 4 07 19 PM" src="https://github.com/user-attachments/assets/b2d34cc7-7cc0-48b5-9f24-4c2e909d434e" />


---

## Install

### Homebrew (macOS/Linux)

```sh
brew install coingecko/coingecko-cli/cg
```

Or tap first, then install:

```sh
brew tap coingecko/coingecko-cli
brew install cg
```

### npm

```sh
npm install -g @coingecko/cg
```

Works on macOS, Linux, and Windows (x64 and arm64). Requires Node.js >= 20. The right native binary for your platform is fetched via npm `optionalDependencies`; no build step.

### Shell script

```sh
curl -sSfL https://raw.githubusercontent.com/coingecko/coingecko-cli/main/install.sh | sh
```

### Go install

```sh
go install github.com/coingecko/coingecko-cli@latest
```

This installs a binary named `coingecko-cli` (matching the module path). To use it as `cg`, add an alias to your shell rc:

```sh
alias cg=coingecko-cli
```

Or symlink it onto your `$PATH`:

```sh
ln -s "$(go env GOBIN 2>/dev/null || echo "$(go env GOPATH)/bin")/coingecko-cli" /usr/local/bin/cg
```

### Manual

Download the binary for your platform from [Releases](https://github.com/coingecko/coingecko-cli/releases), extract, and place `cg` in your `$PATH`.

---

## Setup

Get a free API key at [coingecko.com/en/api](https://www.coingecko.com/en/api), then run:

```sh
# Interactive setup (recommended — input is masked)
cg auth

# Or via environment variables (avoids shell history exposure)
CG_API_KEY=YOUR_API_KEY CG_API_TIER=demo cg auth

# Or with flags (note: key may be visible in shell history and process listings)
cg auth --key YOUR_API_KEY --tier demo
```

Tiers: `demo` (free, public API), `paid` (pro API with full historical data and extra endpoints)

```sh
# Verify configuration
cg status
```

---

## Global Flags

```sh
# JSON output (for scripting and automation)
cg price --ids bitcoin -o json
cg markets --total 10 -o json | jq '.[0].name'

# Dry-run mode (shows the API request without executing it)
cg price --ids bitcoin --dry-run
```

All data commands support `-o json` / `--output json` for machine-readable output. Diagnostics and warnings are written to stderr, so stdout is always clean data.

---

## Commands

### `cg price` — Live Coin Prices

Fetch the current price of one or more coins. Supports both CoinGecko IDs and ticker symbols.

> **Tip:** Find coin IDs by browsing the respective [CoinGecko coin page](https://www.coingecko.com/en/coins/bitcoin) and copying the 'API ID'. You can also get the full list of coin IDs via this [endpoint](https://docs.coingecko.com/reference/coins-list) or [Google Sheet](https://docs.google.com/spreadsheets/d/1wTTuxXt8n9q7C4NDXqQpI3wpKu1_5bGVmP9Xz0XGSyU/edit?gid=0#gid=0).

```sh
# By CoinGecko ID
cg price --ids bitcoin,ethereum

# By ticker symbol (for coins that share the same symbol, the coin with highest market cap will be prioritised)
cg price --symbols btc,eth

# Different target currency
cg price --ids bitcoin --vs eur
```

#### Supported Currencies
For `--vs` currency, refer below for the full supported currency IDs. By default, it will always lookup by `usd`
| IDs | Currency Name | Type |
| :--- | :--- | :--- |
| `usd` | US Dollar | Fiat |
| `eur` | Euro | Fiat |
| `eth` | Ether | Crypto |
| `xau` | Gold - Troy Ounce | Commodity |

<details>
<summary>Click to see full list of supported IDs</summary>
  
| IDs | Currency Name | Type |
| :--- | :--- | :--- |
| `bch` | Bitcoin Cash | Crypto |
| `bnb` | Binance Coin | Crypto |
| `eos` | EOS | Crypto |
| `xrp` | XRP | Crypto |
| `xlm` | Stellar | Crypto |
| `link` | Chainlink | Crypto |
| `dot` | Polkadot | Crypto |
| `yfi` | yearn.finance | Crypto |
| `sol` | Solana | Crypto |
| `usd` | US Dollar | Fiat |
| `aed` | UAE Dirham | Fiat |
| `ars` | Argentine Peso | Fiat |
| `aud` | Australian Dollar | Fiat |
| `bdt` | Bangladeshi Taka | Fiat |
| `bhd` | Bahraini Dinar | Fiat |
| `bmd` | Bermudian Dollar | Fiat |
| `brl` | Brazil Real | Fiat |
| `cad` | Canadian Dollar | Fiat |
| `chf` | Swiss Franc | Fiat |
| `clp` | Chilean Peso | Fiat |
| `cny` | Chinese Yuan | Fiat |
| `czk` | Czech Koruna | Fiat |
| `dkk` | Danish Krone | Fiat |
| `eur` | Euro | Fiat |
| `gbp` | British Pound Sterling | Fiat |
| `gel` | Georgian Lari | Fiat |
| `hkd` | Hong Kong Dollar | Fiat |
| `huf` | Hungarian Forint | Fiat |
| `idr` | Indonesian Rupiah | Fiat |
| `ils` | Israeli New Shekel | Fiat |
| `inr` | Indian Rupee | Fiat |
| `jpy` | Japanese Yen | Fiat |
| `krw` | South Korean Won | Fiat |
| `kwd` | Kuwaiti Dinar | Fiat |
| `lkr` | Sri Lankan Rupee | Fiat |
| `mmk` | Burmese Kyat | Fiat |
| `mxn` | Mexican Peso | Fiat |
| `myr` | Malaysian Ringgit | Fiat |
| `ngn` | Nigerian Naira | Fiat |
| `nok` | Norwegian Krone | Fiat |
| `nzd` | New Zealand Dollar | Fiat |
| `php` | Philippine Peso | Fiat |
| `pkr` | Pakistani Rupee | Fiat |
| `pln` | Polish Zloty | Fiat |
| `rub` | Russian Ruble | Fiat |
| `sar` | Saudi Riyal | Fiat |
| `sek` | Swedish Krona | Fiat |
| `sgd` | Singapore Dollar | Fiat |
| `thb` | Thai Baht | Fiat |
| `try` | Turkish Lira | Fiat |
| `twd` | New Taiwan Dollar | Fiat |
| `uah` | Ukrainian Hryvnia | Fiat |
| `vef` | Venezuelan Bolívar Fuerte | Fiat |
| `vnd` | Vietnamese Dong | Fiat |
| `zar` | South African Rand | Fiat |
| `xdr` | IMF Special Drawing Rights | Other |
| `xag` | Silver - Troy Ounce | Commodity |
| `xau` | Gold - Troy Ounce | Commodity |
| `bits` | Bits (Bitcoin) | Unit |
| `sats` | Satoshis (Bitcoin) | Unit |

</details>

**Output columns:** Coin | Price | 24h Change

---

### `cg markets` — Top Coins by Market Cap

Fetch ranked market data with automatic pagination. The API is queried in 250-coin pages, so `--total 1000` makes exactly 4 API calls.

```sh
cg markets
cg markets --total 100
cg markets --total 500 --vs eur
cg markets --total 250 --order gecko_desc
cg markets --total 250 --export data.csv
```

| Flag | Default | Description |
|---|---|---|
| `--total` | `100` | Number of coins to fetch |
| `--vs` | `usd` | Quote currency |
| `--order` | `market_cap_desc` | Sort order (e.g. `volume_desc`, `gecko_desc`) |
| `--category` | — | Filter by category ID (e.g. `layer-2`, `decentralized-finance-defi`) |
| `--export` | — | Export to CSV file path |

**Output columns:** # | Name | Symbol | Price | Market Cap | Volume | 24h Change

---

### `cg search` — Search Coins

Search for any coin by name or symbol. Returns the top matches with their CoinGecko coin IDs.

```sh
cg search solana
cg search dog --limit 5
```

| Flag | Default | Description |
|---|---|---|
| `--limit` | `10` | Max results to show |

**Output columns:** Rank | Name | Symbol | ID

---

### `cg trending` — Trending (24h)

Shows the three trending tables in one view:

```sh
cg trending
```

- **Top 15 trending coins** — with market cap rank
- **Top 7 trending NFTs** — with floor price change
- **Top 6 trending categories** — with market cap change

---

### `cg history` — Historical Price Data

Three modes for querying historical data. All modes support `--vs` currency and `--export`.

**Single date snapshot:**
```sh
cg history bitcoin --date 2024-01-15
cg history ethereum --date 2024-06-01 --vs eur
```

**Past N days (price data):**
```sh
cg history bitcoin --days 7
cg history bitcoin --days 30 --export btc_30d.csv
cg history bitcoin --days 7 --ohlc          # OHLC candle data instead
```

**Custom date range:**
```sh
cg history bitcoin --from 2024-01-01 --to 2024-06-30
cg history bitcoin --from 2024-01-01 --to 2024-03-31 --export q1.csv
cg history bitcoin --from 2024-01-01 --to 2024-06-30 --ohlc   # OHLC output
```

| Flag | Description |
|---|---|
| `--date YYYY-MM-DD` | Single-day snapshot (price, market cap, volume) |
| `--days N` | Past N days of price data (any integer, or `max`) |
| `--from / --to YYYY-MM-DD` | Inclusive date range (price data) |
| `--ohlc` | Switch `--days` or `--from/--to` to OHLC output |
| `--vs` | Quote currency (default: `usd`) |
| `--interval` | Data granularity: `daily`, `hourly`, or `5m` |
| `--export` | Export to CSV file path |

Note: `hourly` and `5m` interval data are only available from **February 2, 2018** onwards. `5m` interval is exclusive for [Enterprise](https://www.coingecko.com/en/api/pricing) plan only. 

---

### `cg top-gainers-losers` — Top Movers (Exclusive for [Analyst plan](https://www.coingecko.com/en/api/pricing) & above)

```sh
cg top-gainers-losers
cg top-gainers-losers --losers --duration 7d
cg top-gainers-losers --top-coins 300 --export gainers.csv
```

| Flag | Default | Description |
|---|---|---|
| `--vs` | `usd` | Quote currency |
| `--duration` | `24h` | Time window (1h, 24h, 7d, 14d, 30d, 60d, 1y) |
| `--top-coins` | `1000` | Pool size (300, 500, 1000, all) |
| `--losers` | `false` | Show losers instead of gainers |
| `--export` | — | Export to CSV file path |

---

### `cg watch` — Live Price Streaming (Exclusive for [Analyst plan](https://www.coingecko.com/en/api/pricing) & above)

Stream real-time price updates via CoinGecko's WebSocket API. USD prices only.

```sh
cg watch --ids bitcoin,ethereum          # Live updating table
cg watch --symbols btc,eth               # Resolve symbols, then stream
cg watch --ids bitcoin -o json           # NDJSON output (pipe-friendly)
cg watch --ids bitcoin -o json | jq .price  # Stream prices with jq
cg watch --ids bitcoin --dry-run         # Show WebSocket request info
```

| Flag | Default | Description |
|---|---|---|
| `--ids` | — | Comma-separated coin IDs |
| `--symbols` | — | Comma-separated symbols (resolved to IDs) |

**Table mode** (default): clears screen and re-renders on each update. Press `Ctrl+C` to quit.

**JSON mode** (`-o json`): one JSON object per line per update to stdout. Exits cleanly on broken pipe.

---

### `cg update` — Upgrade the CLI

Check for a new version and upgrade in one step. Auto-detects whether you installed via Homebrew, `go install`, or the install script, and hands off to the right tool.

```sh
cg update

# Override install method if auto-detection gets it wrong
cg update --method homebrew   # or: go, script
```

The CLI also checks for updates on launch (cached 24h) and shows a reminder if you're behind. Set `CG_NO_UPDATE_CHECK=1` to disable this in CI.

---

## Category Filtering

CoinGecko tracks 500+ categories including Real World Assets, commodities, and tokenized stocks. Use the `--category` flag to filter:

```sh
cg markets --category tokenized-gold              # Gold & Silver pegged assets
cg markets --category real-world-assets-rwa        # Real Estate & T-Bills
cg markets --category artificial-intelligence      # Top AI coins
cg markets --category layer-2 --export l2.csv      # Export all L2 tokens
cg tui markets --category solana-meme-coins        # Browse in TUI mode
```

The `--category` flag works in both `cg markets` and `cg tui markets`. In TUI mode, the active category is displayed in the header.

> **Tip:** Find category IDs by browsing the [CoinGecko categories page](https://www.coingecko.com/en/categories) and copying the ID from the URL. You can also get the full list via this [endpoint](https://docs.coingecko.com/reference/coins-categories-list) or [Google Sheet](https://docs.google.com/spreadsheets/d/1wTTuxXt8n9q7C4NDXqQpI3wpKu1_5bGVmP9Xz0XGSyU/edit?gid=214581757#gid=214581757).

---

## CSV Export

`markets` and `history` commands can export raw data to CSV for analysis in Excel, Python, etc.

```sh
cg markets --total 500 --export top500.csv
cg history bitcoin --days 30 --export btc_30d.csv
cg history bitcoin --from 2024-01-01 --to 2024-12-31 --export btc_2024.csv
cg top-gainers-losers --export gainers.csv
```

CSV files contain raw numbers (not formatted strings), making them directly usable in data pipelines.

---

## Interactive TUI

### `cg tui markets` — Top Market Cap Ranked Coins

```sh
cg tui markets
cg tui markets --category layer-1
```

Launches a live interactive table of the top 50 coins by market cap.

### `cg tui trending` — Trending Coins

```sh
cg tui trending
```

Launches a live interactive table of the top 15 trending coins (up to 30 on paid plans).

### Keyboard Controls

| Key | Action |
|---|---|
| `j` / `↓` | Move selection down |
| `k` / `↑` | Move selection up |
| `Enter` | Open detail view |
| `Esc` / `Backspace` | Back to list |
| `q` / `Ctrl+C` | Quit |

### Detail View

Pressing `Enter` on any coin opens a split-panel detail view:

**Left panel** — Key metrics: price, 24h change, high/low, market cap, volume, ATH/ATL, circulating and total supply.

**Right panel** — 7-day price chart rendered as a braille-dot line graph in the terminal.


---

## Full Command Reference

```
cg [command]

Commands:
  auth                 Save your CoinGecko API key and tier (demo/paid)
  status               Show current auth configuration
  price                Get the current price of one or more coins
  markets              List top coins by market cap
  search               Search for coins by name or symbol
  trending             Show trending coins, NFTs, and categories (24h)
  history              Get historical price data for a coin
  top-gainers-losers   Show top gaining and losing coins (paid plans only)
  watch                Stream live coin prices via WebSocket (analyst or above)
  tui                  Interactive terminal UI (markets, trending)
  update               Upgrade the CLI to the latest version
  commands             List all commands with API metadata (for agents/LLMs)
  help                 Print help for a command

Global Flags:
  -o, --output string  Output format: table, json (default "table")
      --dry-run        Show the API request without executing it
  -h, --help           Print help
  -v, --version        Print version
```

For per-command help:

```sh
cg price --help
cg markets --help
cg history --help
```

---

## Development

```sh
# Build
make build

# Test
make test

# Lint (requires golangci-lint)
make lint
```

## Tech Stack

| Package | Purpose |
|---|---|
| [cobra](https://github.com/spf13/cobra) | CLI framework and command routing |
| [viper](https://github.com/spf13/viper) | Configuration management |
| [bubbletea](https://github.com/charmbracelet/bubbletea) | Interactive TUI framework |
| [lipgloss](https://github.com/charmbracelet/lipgloss) | Terminal styling and layout |
| [huh](https://github.com/charmbracelet/huh) | Interactive auth prompts |
| [ntcharts](https://github.com/NimbleMarkets/ntcharts) | Braille terminal charts |
| [gorilla/websocket](https://github.com/gorilla/websocket) | WebSocket client for live price streaming |
| [goreleaser](https://goreleaser.com) | Cross-platform release builds |

## License

MIT — see [LICENSE](LICENSE).

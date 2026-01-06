# Questrade-YNAB CLI

A Go CLI application to fetch investment account balances from Questrade and update corresponding accounts in YNAB (You Need A Budget).

## Features

- Authenticate with Questrade and YNAB APIs using personal access tokens
- Fetch current investment account balances from Questrade
- Update account balances in YNAB
- Interactive account mapping between Questrade and YNAB accounts
- Preview all changes before applying them
- Dry run mode to see what would be updated without making changes
- Approval step with detailed change information
- Lists and saves all fetched accounts and mappings to JSON files for lookup

## Commands

### `auth set` / `auth login`
Set up and authenticate your Questrade and YNAB credentials. Prompts for tokens and budget ID, and saves them to your config.

### `mapping set`
Interactive mapping setup. Guides you through selecting Questrade accounts and mapping them to YNAB accounts. Saves the mapping as a flat JSON object in `~/.questrade-ynab/mappings.json`.

### `mapping list`
Lists all Questrade and YNAB accounts with their names and balances. Also displays which Questrade account is mapped to which YNAB account (by name). Writes all fetched accounts to JSON files for lookup.

### `sync`
Fetches latest balances from Questrade and prepares updates for mapped YNAB accounts. Shows a detailed preview of changes, including current and new balances and the difference. Asks for approval before applying updates.

#### `sync --dry-run`
Shows the preview of changes without making any updates.

## Configuration

Configuration is stored in `~/.questrade-ynab/config.json` and includes:
- Questrade personal access token
- Questrade API server URL
- YNAB access token
- YNAB budget ID

Account mappings are stored in `~/.questrade-ynab/mappings.json` as a flat JSON object:
```json
{
  "QUESTRADE_ACCOUNT_NUMBER": "YNAB_ACCOUNT_ID"
}
```

Fetched accounts are saved to:
- `~/.questrade-ynab/questrade_accounts.json`
- `~/.questrade-ynab/ynab_accounts.json`

## Usage

### List Accounts and Mappings

```bash
./questrade-ynab mapping list
```
Displays all Questrade and YNAB accounts with balances, and shows the current account mappings.

### Set Up Account Mapping

```bash
./questrade-ynab mapping set
```
Guides you through mapping Questrade accounts to YNAB accounts interactively.

### Sync Account Balances

```bash
./questrade-ynab sync
```
Fetches balances from Questrade and updates mapped YNAB accounts after preview and approval.

### Dry Run

```bash
./questrade-ynab sync --dry-run
```
Shows what would be updated without making any changes.

## API Documentation

- [Questrade API Documentation](https://www.questrade.com/api/documentation/getting-started)
- [YNAB API Documentation](https://api.ynab.com/)

## Notes

- YNAB uses "milliunits" for currency amounts (1000 milliunits = 1 unit)
- Questrade personal access tokens are valid for 7 days
- YNAB access tokens do not expire but can be revoked
- Keep your tokens secure and never commit them to version control

## Troubleshooting

- **Config file not found:** Run `questrade-ynab auth set` to create the configuration file.
- **Error fetching Questrade accounts:** Your Questrade token may have expired. Generate a new one and run `auth set`.
- **Resource not found errors:** Verify your YNAB budget ID.
- **Account not syncing:** Use `mapping list` to verify mappings, or run `sync --dry-run` to preview updates.

## License

MIT

## Build from Source

### Prerequisites

- Go 1.21 or higher
- A Questrade account with API access enabled
- A YNAB account with a personal access token

### Build

```bash
git clone https://github.com/brymastr/questrade-ynab.git
cd questrade-ynab
go build -o questrade-ynab
```

# Questrade → YNAB

Sync investment account balances from Questrade into YNAB (You Need A Budget).
Runs three ways from the same codebase:

- **CLI** — the original command-line tool (`auth`, `mapping`, `sync`).
- **Web app (local)** — a Go API + React UI you run with `make dev`.
- **Web app (AWS)** — the same app deployed serverlessly (Lambda + CloudFront + DynamoDB).

See [Web app](#web-app) and [Deploy to AWS](#deploy-to-aws) below; the CLI docs follow.

## Features

- Authenticate with Questrade and YNAB APIs using personal access tokens
- Fetch current investment account balances from Questrade
- Update account balances in YNAB
- Interactive account mapping between Questrade and YNAB accounts
- Preview all changes before applying them
- Dry run mode to see what would be updated without making changes
- Approval step with detailed change information
- Lists and saves all fetched accounts and mappings to JSON files for lookup

## Web app

The project also runs as a web app: a Go HTTP API (chi) plus a React/Vite
frontend. You connect Questrade (paste a refresh token — Questrade personal apps
don't support interactive OAuth), connect YNAB (OAuth), map accounts, preview the
transactions a sync would create, then apply them.

### Local development

Requirements: Go 1.25+, Node 18+.

1. Configure credentials:
   ```sh
   cp .env.example .env
   # set QUESTRADE_CLIENT_ID/SECRET and YNAB_CLIENT_ID/SECRET
   ```
   Register the YNAB redirect URI `http://localhost:5173/auth/ynab/callback` in
   your YNAB app at https://app.ynab.com/settings/developer.
2. Install frontend deps and run both servers:
   ```sh
   make install
   make dev     # backend :8080 (live-reload via air) + frontend :5173
   ```
3. Open http://localhost:5173.

Local runs use SQLite at `./data/app.db` — no external services needed.

## Deploy to AWS

The web app deploys serverlessly: a Go **Lambda** (arm64) behind a **CloudFront**
distribution, with the SPA on **S3**, data in **DynamoDB**, and credentials in
**Secrets Manager**. Infrastructure is AWS CDK in [`infra/`](infra/README.md).

Each environment (`dev`, `prod`, …) is a **separate stack** (`QuestradeYnab-<env>`)
with its own domain, table, and secret — selected with `APP_ENV` (default `dev`).

### Prerequisites

- AWS CLI configured (`aws sts get-caller-identity`) and an existing Route53 hosted zone.
- One-time per account:
  ```sh
  cd infra && npm install && npx cdk bootstrap aws://<account-id>/us-east-1 && cd ..
  ```

### Configure an environment

```sh
cp infra/.env.example infra/.env.dev    # set DOMAIN_NAME, HOSTED_ZONE_NAME
```

Precedence: shell env > `infra/.env.<env>` > `infra/.env` (shared defaults).

### Deploy (backend + frontend, one command)

```sh
make deploy               # dev
APP_ENV=prod make deploy  # prod, once infra/.env.prod exists
```

`make deploy` cross-compiles the arm64 Lambda, builds the SPA, then runs
`cdk deploy` — which ships the Lambda **and** uploads `web/dist` to S3 and
invalidates CloudFront. First deploy takes ~15–25 min (CloudFront + ACM).

### After the first deploy

1. Fill the four OAuth values into the `questrade-ynab/<env>` Secrets Manager
   secret (`JWT_SECRET` is auto-generated).
2. Register `https://<your-domain>/auth/ynab/callback` in your YNAB app.
3. Open `https://<your-domain>` and connect.

Full detail (secrets, DNS, teardown) is in [`infra/README.md`](infra/README.md).

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

- Go 1.25 or higher
- A Questrade account with API access enabled
- A YNAB account with a personal access token

### Build

```bash
git clone https://github.com/brymastr/questrade-ynab.git
cd questrade-ynab
go build -o questrade-ynab
```

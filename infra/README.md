# Infrastructure (AWS CDK)

Deploys the app as: **CloudFront** (custom domain, ACM) → **S3** (SPA) + **Lambda**
(Go, arm64, Function URL) → **DynamoDB**. Everything is in **us-east-1** (required
for the CloudFront ACM cert).

## One-time setup

```sh
cd infra
npm install
npx cdk bootstrap aws://<account-id>/us-east-1   # if this account/region isn't bootstrapped
```

You also need an existing Route53 **hosted zone** for your domain.

## Deploy

### Environments

Each environment (dev, prod, …) is a **separate stack** (`QuestradeYnab-<env>`)
with its own DynamoDB table, Lambda, CloudFront, domain, and secret. Select one
with `APP_ENV` (default `dev`).

Put each environment's config in `infra/.env.<env>` (see `infra/.env.example`):

```sh
cp infra/.env.example infra/.env.dev
# edit DOMAIN_NAME (e.g. qy-dev.example.com), HOSTED_ZONE_NAME
```

Precedence: shell env vars > `infra/.env.<env>` > `infra/.env` (shared defaults).
So values common to all envs (e.g. `HOSTED_ZONE_NAME`) can live in `infra/.env`.
Your AWS account/region come from your AWS credentials automatically (region is
pinned to us-east-1 for the CloudFront cert).

### Deploy

Build the artifacts and deploy (from the repo root):

```sh
make deploy               # deploys the dev stack (APP_ENV defaults to dev)
APP_ENV=prod make deploy  # later, once infra/.env.prod exists
```

`make deploy` builds the arm64 Lambda + frontend, then runs `cdk deploy` for the
selected environment's stack only.

The stack **creates** a per-environment Secrets Manager secret (`SECRETS_NAME`,
default `questrade-ynab/<env>`) with an auto-generated `JWT_SECRET` and blank OAuth
fields. No plaintext is in the template; the Lambda reads it at cold start and its
role can read only that secret.

## After first deploy

1. Fill in the YNAB OAuth values in the secret (JWT_SECRET is already generated;
   the QUESTRADE_* keys are unused and can stay blank).
   Easiest via the console, or fetch-merge-put:
   ```sh
   aws secretsmanager put-secret-value --secret-id questrade-ynab/dev \
     --secret-string '{
       "JWT_SECRET": "<keep the generated value>",
       "YNAB_CLIENT_ID": "...", "YNAB_CLIENT_SECRET": "..." }'
   ```
   (`put-secret-value` replaces the whole JSON, so include the existing
   `JWT_SECRET`.) Running instances keep the old values, so force fresh ones
   with any config update, e.g.:
   ```sh
   aws lambda update-function-configuration --function-name <Api function> \
     --description "secrets reloaded $(date -u +%FT%TZ)"
   ```
2. In YNAB developer settings, add the redirect URI
   `https://<DOMAIN_NAME>/auth/ynab/callback`.
3. Open `https://<DOMAIN_NAME>`, connect YNAB (OAuth) and Questrade (paste token).

## Notes / follow-ups

- **Secrets** live in Secrets Manager; the Lambda fetches them at cold start via
  `SECRETS_ARN` and its execution role has `secretsmanager:GetSecretValue` on that
  secret only. No plaintext in the template or function config.
- **Users** are identified by their Questrade user ID (linked on first connect).
  `ALLOWED_QUESTRADE_USER_IDS` in `infra/.env.<env>` restricts who can connect;
  leave it empty to open the app to anyone.
- The Lambda **Function URL is public** (`AuthType.NONE`), because OAC/IAM signing
  doesn't sign request bodies for Lambda URLs and breaks every POST/PUT. Instead
  CloudFront sends an `x-origin-verify` header from the `OriginVerify` secret, and
  the function returns 403 to HTTP requests without it.
- **Scheduled sync**: EventBridge Scheduler ticks every 5 minutes and the function
  queues due users on the `SyncJobs` SQS queue, processed at most 5 at a time.
  Jobs that crash twice land in `SyncJobsDlq`.

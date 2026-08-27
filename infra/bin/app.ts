#!/usr/bin/env node
import 'source-map-support/register';
import * as path from 'path';
import * as dotenv from 'dotenv';
import * as cdk from 'aws-cdk-lib';
import { QuestradeYnabStack } from '../lib/questrade-ynab-stack';

// Pick the environment (dev/prod/...) and load its config. Precedence:
//   real shell env vars  >  infra/.env.<APP_ENV>  >  infra/.env (shared defaults)
// dotenv never overwrites an already-set var, so load specific-first.
const envName = process.env.APP_ENV ?? 'dev';
const loadEnv = (file: string) => dotenv.config({ path: path.join(__dirname, '..', file) });
loadEnv(`.env.${envName}`); // environment-specific (wins)
loadEnv('.env'); // shared defaults (fills the rest)

const app = new cdk.App();

// Required deploy-time config (from infra/.env.<env> or the shell):
//   DOMAIN_NAME       e.g. qy-dev.example.com  (the app's public hostname)
//   HOSTED_ZONE_NAME  e.g. example.com         (an existing Route53 hosted zone)
const domainName = process.env.DOMAIN_NAME;
const hostedZoneName = process.env.HOSTED_ZONE_NAME;
if (!domainName || !hostedZoneName) {
  throw new Error(`DOMAIN_NAME and HOSTED_ZONE_NAME are required (APP_ENV=${envName})`);
}

// Secrets Manager secret the stack creates (per-environment by default).
const secretName = process.env.SECRETS_NAME ?? `questrade-ynab/${envName}`;

// One stack per environment, so dev and prod are fully separate resources.
new QuestradeYnabStack(app, `QuestradeYnab-${envName}`, {
  // CloudFront ACM certificates must live in us-east-1, so deploy the whole
  // stack there (DynamoDB/Lambda in us-east-1 is fine).
  env: { account: process.env.CDK_DEFAULT_ACCOUNT, region: 'us-east-1' },
  envName,
  domainName,
  hostedZoneName,
  secretName,
});

import * as path from 'path';
import * as cdk from 'aws-cdk-lib';
import { Construct } from 'constructs';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as s3 from 'aws-cdk-lib/aws-s3';
import * as s3deploy from 'aws-cdk-lib/aws-s3-deployment';
import * as cloudfront from 'aws-cdk-lib/aws-cloudfront';
import * as origins from 'aws-cdk-lib/aws-cloudfront-origins';
import * as acm from 'aws-cdk-lib/aws-certificatemanager';
import * as route53 from 'aws-cdk-lib/aws-route53';
import * as targets from 'aws-cdk-lib/aws-route53-targets';
import * as secretsmanager from 'aws-cdk-lib/aws-secretsmanager';
import * as scheduler from 'aws-cdk-lib/aws-scheduler';
import * as schedulerTargets from 'aws-cdk-lib/aws-scheduler-targets';
import * as sqs from 'aws-cdk-lib/aws-sqs';
import * as kms from 'aws-cdk-lib/aws-kms';
import * as logs from 'aws-cdk-lib/aws-logs';
import { SqsEventSource } from 'aws-cdk-lib/aws-lambda-event-sources';

export interface QuestradeYnabStackProps extends cdk.StackProps {
  // Environment name (e.g. "dev", "prod") — used for tagging.
  envName: string;
  domainName: string;
  hostedZoneName: string;
  // Name for the Secrets Manager secret this stack creates. It holds a JSON
  // object with keys JWT_SECRET (auto-generated), QUESTRADE_CLIENT_ID,
  // QUESTRADE_CLIENT_SECRET, YNAB_CLIENT_ID, YNAB_CLIENT_SECRET. The OAuth values
  // start blank and are filled in after the first deploy (no plaintext in the
  // template).
  secretName: string;
  // Comma-separated Questrade user IDs allowed to connect; empty = anyone.
  allowedQuestradeUserIds: string;
}

export class QuestradeYnabStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: QuestradeYnabStackProps) {
    super(scope, id, props);

    cdk.Tags.of(this).add('app', 'questrade-ynab');
    cdk.Tags.of(this).add('env', props.envName);

    const appUrl = `https://${props.domainName}`;

    // --- DynamoDB single table ---
    const table = new dynamodb.TableV2(this, 'Table', {
      partitionKey: { name: 'PK', type: dynamodb.AttributeType.STRING },
      sortKey: { name: 'SK', type: dynamodb.AttributeType.STRING },
      billing: dynamodb.Billing.onDemand(),
      // Session items carry `ttl` (epoch seconds) so DynamoDB deletes them after expiry.
      timeToLiveAttribute: 'ttl',
      removalPolicy: cdk.RemovalPolicy.RETAIN,
    });

    // --- App secrets (created here; fetched at Lambda cold start) ---
    // JWT_SECRET is auto-generated. The OAuth values start blank — fill them in
    // after the first deploy via the console or `aws secretsmanager put-secret-value`.
    const secret = new secretsmanager.Secret(this, 'AppSecrets', {
      secretName: props.secretName,
      description: 'Questrade->YNAB app credentials',
      generateSecretString: {
        secretStringTemplate: JSON.stringify({
          QUESTRADE_CLIENT_ID: '',
          QUESTRADE_CLIENT_SECRET: '',
          YNAB_CLIENT_ID: '',
          YNAB_CLIENT_SECRET: '',
        }),
        generateStringKey: 'JWT_SECRET',
        excludePunctuation: true,
        passwordLength: 48,
      },
      removalPolicy: cdk.RemovalPolicy.RETAIN,
    });

    // Encrypts users' OAuth tokens before they're written to DynamoDB. Only the
    // Lambda is granted use of it, and every use is logged in CloudTrail.
    // Retained on stack deletion: losing it makes stored tokens unreadable.
    const tokenKey = new kms.Key(this, 'TokenKey', {
      description: 'questrade-ynab OAuth token encryption',
      enableKeyRotation: true,
      removalPolicy: cdk.RemovalPolicy.RETAIN,
    });

    // Shared secret CloudFront sends to the Lambda origin, so requests that
    // skip CloudFront and hit the public Function URL directly are refused.
    // CloudFormation resolves it via a dynamic reference (not in the template).
    const originVerify = new secretsmanager.Secret(this, 'OriginVerify', {
      description: 'Header value CloudFront sends to the API origin',
      generateSecretString: { excludePunctuation: true, passwordLength: 48 },
    });

    // Function logs (request paths, sync outcomes, account numbers in warnings)
    // expire after a month rather than accumulating forever.
    const apiLogs = new logs.LogGroup(this, 'ApiLogs', {
      retention: logs.RetentionDays.ONE_MONTH,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    // --- Lambda (Go, arm64, custom runtime) ---
    // `assets/bootstrap` is produced by `make build-lambda`. Only the secret ARN
    // (non-sensitive) is in the env; the Lambda reads the values at runtime.
    const fn = new lambda.Function(this, 'Api', {
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      code: lambda.Code.fromAsset(path.join(__dirname, '..', 'assets')),
      memorySize: 256,
      logGroup: apiLogs,
      // One user's sync, or one tick enqueueing due users. HTTP requests are
      // capped lower by CloudFront's origin timeout.
      timeout: cdk.Duration.seconds(60),
      // Scheduler invokes asynchronously; don't let Lambda re-run a tick that
      // errored or timed out partway (HTTP requests are synchronous, unaffected).
      retryAttempts: 0,
      environment: {
        DB_BACKEND: 'dynamo',
        DYNAMO_TABLE: table.tableName,
        APP_URL: appUrl,
        CORS_ORIGIN: appUrl,
        COOKIE_SECURE: 'true',
        YNAB_REDIRECT_URI: `${appUrl}/auth/ynab/callback`,
        SECRETS_ARN: secret.secretArn,
        ORIGIN_VERIFY_SECRET_ARN: originVerify.secretArn,
        TOKEN_KMS_KEY_ID: tokenKey.keyArn,
        ALLOWED_QUESTRADE_USER_IDS: props.allowedQuestradeUserIds,
      },
    });
    table.grantReadWriteData(fn);
    secret.grantRead(fn);
    originVerify.grantRead(fn);
    tokenKey.grantEncryptDecrypt(fn);

    // --- Scheduled sync ---
    // Lambda has no long-lived cron. EventBridge Scheduler ticks every 5
    // minutes; the function enqueues one job per user whose slot (spread over
    // 4:30-7:30 PM Eastern) fell in that interval, and the queue runs them with
    // a concurrency cap so a busy slot can't flood Questrade or YNAB. `source`
    // and the 5-minute interval must match lambda_sync.go.
    const syncDlq = new sqs.Queue(this, 'SyncJobsDlq', {
      retentionPeriod: cdk.Duration.days(14),
    });
    const syncQueue = new sqs.Queue(this, 'SyncJobs', {
      // At least 6x the function timeout, per the SQS event source guidance.
      visibilityTimeout: cdk.Duration.minutes(6),
      // Only crashes/timeouts are retried; sync errors are recorded and acked.
      deadLetterQueue: { queue: syncDlq, maxReceiveCount: 2 },
    });
    syncQueue.grantSendMessages(fn);
    fn.addEnvironment('SYNC_QUEUE_URL', syncQueue.queueUrl);
    fn.addEventSource(new SqsEventSource(syncQueue, {
      batchSize: 1,
      maxConcurrency: 5,
      reportBatchItemFailures: true,
    }));

    new scheduler.Schedule(this, 'SyncTick', {
      description: 'Every 5 minutes: enqueue due Questrade->YNAB syncs',
      schedule: scheduler.ScheduleExpression.cron({ minute: '0/5' }),
      target: new schedulerTargets.LambdaInvoke(fn, {
        input: scheduler.ScheduleTargetInput.fromObject({
          source: 'questrade-ynab.scheduler',
          scheduled_time: scheduler.ContextAttribute.scheduledTime,
        }),
        // A retried tick would enqueue the same slot twice.
        retryAttempts: 0,
      }),
    });

    // Public Function URL (no IAM). OAC signs SigV4 but doesn't include the
    // request body for Lambda URLs, so every POST/PUT fails with a signature
    // mismatch; unsigned avoids that. Instead CloudFront adds x-origin-verify
    // and the function rejects HTTP requests without it.
    const fnUrl = fn.addFunctionUrl({ authType: lambda.FunctionUrlAuthType.NONE });

    // --- Static site bucket (private, CloudFront OAC only) ---
    const bucket = new s3.Bucket(this, 'SiteBucket', {
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      encryption: s3.BucketEncryption.S3_MANAGED,
      removalPolicy: cdk.RemovalPolicy.RETAIN,
    });

    // --- TLS cert (in-region us-east-1 for CloudFront) ---
    const zone = route53.HostedZone.fromLookup(this, 'Zone', {
      domainName: props.hostedZoneName,
    });
    const cert = new acm.Certificate(this, 'Cert', {
      domainName: props.domainName,
      validation: acm.CertificateValidation.fromDns(zone),
    });

    // --- CloudFront: S3 for the SPA, Lambda for /api and /auth (same origin) ---
    const apiBehavior: cloudfront.BehaviorOptions = {
      origin: new origins.FunctionUrlOrigin(fnUrl, {
        customHeaders: { 'x-origin-verify': originVerify.secretValue.unsafeUnwrap() },
      }),
      viewerProtocolPolicy: cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
      allowedMethods: cloudfront.AllowedMethods.ALLOW_ALL,
      cachePolicy: cloudfront.CachePolicy.CACHING_DISABLED,
      originRequestPolicy: cloudfront.OriginRequestPolicy.ALL_VIEWER_EXCEPT_HOST_HEADER,
    };

    const distribution = new cloudfront.Distribution(this, 'Cdn', {
      defaultBehavior: {
        origin: origins.S3BucketOrigin.withOriginAccessControl(bucket),
        viewerProtocolPolicy: cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
      },
      additionalBehaviors: {
        '/api/*': apiBehavior,
        '/auth/*': apiBehavior,
      },
      domainNames: [props.domainName],
      certificate: cert,
      defaultRootObject: 'index.html',
      // No SPA error-response fallback: the app uses HashRouter (only "/" is ever
      // requested), and a distribution-wide 403/404 -> index.html rule would
      // wrongly turn API/auth error responses into HTML.
    });

    // --- Publish the built SPA and invalidate the cache on deploy ---
    new s3deploy.BucketDeployment(this, 'SiteDeploy', {
      sources: [s3deploy.Source.asset(path.join(__dirname, '..', '..', 'web', 'dist'))],
      destinationBucket: bucket,
      distribution,
      distributionPaths: ['/*'],
    });

    // --- DNS aliases into the existing hosted zone ---
    new route53.ARecord(this, 'AliasA', {
      zone,
      recordName: props.domainName,
      target: route53.RecordTarget.fromAlias(new targets.CloudFrontTarget(distribution)),
    });
    new route53.AaaaRecord(this, 'AliasAAAA', {
      zone,
      recordName: props.domainName,
      target: route53.RecordTarget.fromAlias(new targets.CloudFrontTarget(distribution)),
    });

    new cdk.CfnOutput(this, 'Url', { value: appUrl });
    new cdk.CfnOutput(this, 'DistributionId', { value: distribution.distributionId });
    new cdk.CfnOutput(this, 'TableName', { value: table.tableName });
  }
}

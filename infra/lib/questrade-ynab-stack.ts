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

    // --- Lambda (Go, arm64, custom runtime) ---
    // `assets/bootstrap` is produced by `make build-lambda`. Only the secret ARN
    // (non-sensitive) is in the env; the Lambda reads the values at runtime.
    const fn = new lambda.Function(this, 'Api', {
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      code: lambda.Code.fromAsset(path.join(__dirname, '..', 'assets')),
      memorySize: 256,
      timeout: cdk.Duration.seconds(30),
      environment: {
        DB_BACKEND: 'dynamo',
        DYNAMO_TABLE: table.tableName,
        APP_URL: appUrl,
        CORS_ORIGIN: appUrl,
        COOKIE_SECURE: 'true',
        YNAB_REDIRECT_URI: `${appUrl}/auth/ynab/callback`,
        SECRETS_ARN: secret.secretArn,
      },
    });
    table.grantReadWriteData(fn);
    secret.grantRead(fn);

    // IAM-authed Function URL, reachable only via CloudFront (OAC signs requests).
    const fnUrl = fn.addFunctionUrl({ authType: lambda.FunctionUrlAuthType.AWS_IAM });

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
      origin: origins.FunctionUrlOrigin.withOriginAccessControl(fnUrl),
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
      // SPA safety net (the app uses HashRouter, so rarely needed).
      errorResponses: [
        { httpStatus: 403, responseHttpStatus: 200, responsePagePath: '/index.html' },
        { httpStatus: 404, responseHttpStatus: 200, responsePagePath: '/index.html' },
      ],
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

import { Construct } from "constructs";
import * as path from "path";
import {
  aws_iam as iam,
  aws_lambda as lambda,
  aws_events_targets as targets,
  aws_events as events,
} from "aws-cdk-lib";
import * as gobuild from '@aws-cdk/aws-lambda-go-alpha';

export interface DatabaseCollectorProps {
}

export class DatabaseCollector extends Construct {
  private IAMRole() {
    const role = new iam.Role(this, "Role", {
      assumedBy: new iam.ServicePrincipal("lambda.amazonaws.com")
    })

    role.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "secretsmanager:DescribeSecret",
          "secretsmanager:ListSecrets"
        ],
        resources: ["*"]
      })
    )
    role.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "secretsmanager:GetSecretValue",
        ],
        resources: ["*"],
        conditions: {
          "StringEquals": { "database-collector:enabled": "true" }
        }
      })
    )
    role.addManagedPolicy({
      managedPolicyArn: "arn:aws:iam::aws:policy/CloudWatchFullAccessV2"
    })
    role.addManagedPolicy({
      managedPolicyArn: "arn:aws:iam::aws:policy/AmazonPrometheusRemoteWriteAccess"
    })

    return role
  }

  /**
   * buildAndInstallGOLambda build the code and create the lambda
   * @param id - CDK id for this lambda
   * @param lambdaPath - Location of the code
   * @param handler - name of the handler to call for this lambda
   */
  private buildAndInstallGOLambda(id: string, lambdaPath: string, handler: string) {
    const environment = {
      CGO_ENABLED: '0'
    };

    const role = this.IAMRole()

    const scheduleRule = new events.Rule(this, 'Rule', {
      schedule: events.Schedule.expression('cron(*/5 * * * ? *)')
    })

    const lambdaFn = new lambda.Function(this, id, {
      code: lambda.Code.fromAsset(lambdaPath, {
        bundling: {
          image: lambda.Runtime.PROVIDED_AL2023.bundlingImage,
          user: "root",
          environment,
          command: [
            'bash', '-c', [
              'curl -LO https://go.dev/dl/go1.21.5.linux-arm64.tar.gz',
              'rm -rf /usr/local/go && tar -C /usr/local -xzf go1.21.5.linux-arm64.tar.gz',
              'export PATH=$PATH:/usr/local/go/bin',
              'make vendor',
              `make ${process.env.LAMBDA_BUILD_COMMAND || 'lambda-build'}`
            ].join(' && ')
          ]
        }
      }),
      handler,
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      role: role
    })
    scheduleRule.addTarget(new targets.LambdaFunction(lambdaFn))
  }

  private buildAndInstallGoLocal(handler: string, entry: string, version: string){
    const role = this.IAMRole()
    const scheduleRule = new events.Rule(this, 'Rule', {
      schedule: events.Schedule.expression('cron(*/5 * * * ? *)')
    })
    const gofn = new gobuild.GoFunction(this,  handler, {
      entry: entry,
      bundling: {
        user: "root",
        environment: {
          GOOS: process.env.GOOS || "linux",
          GOARCH: process.env.GOARCH || "arm64",
          CGO_ENABLED: "0"
        },
        goBuildFlags: ['-ldflags "-s -w"'],
        buildArgs: {
          VERSION: version
        },
      },
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      role: role
    })
    scheduleRule.addTarget(new targets.LambdaFunction(gofn))
    return gofn
  }
  constructor(scope: Construct, id: string, props: DatabaseCollectorProps) {
    super(scope, id);
    this.buildAndInstallGoLocal("database-collector", path.join(__dirname, "../lambda/"), "0.0.1")
    // this.buildAndInstallGOLambda("database-collector", path.join(__dirname, "../lambda/"), "main")
  }
}

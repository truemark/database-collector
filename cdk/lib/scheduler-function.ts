import {NodejsFunction} from "aws-cdk-lib/aws-lambda-nodejs";
import {Construct} from "constructs";
import {Architecture, Runtime} from "aws-cdk-lib/aws-lambda";
import {Duration, RemovalPolicy} from "aws-cdk-lib";
import * as path from "path";
import {PolicyStatement} from "aws-cdk-lib/aws-iam";
import {Rule, Schedule} from "aws-cdk-lib/aws-events";
import {LambdaFunction} from "aws-cdk-lib/aws-events-targets";
import {LogGroup, RetentionDays} from "aws-cdk-lib/aws-logs";
import {log} from "aws-cdk-lib/custom-resources/lib/provider-framework/runtime/util";

interface SchedulerFunctionProps {
  readonly tagPrefix: string;
}

export class SchedulerFunction extends NodejsFunction {
  constructor(scope: Construct, id: string, props: SchedulerFunctionProps) {
    const logGroup = new LogGroup(scope, `${id}LogGroup`, {
      retention: RetentionDays.ONE_WEEK,
      removalPolicy: RemovalPolicy.DESTROY,
    });
    super(scope, id, {
      runtime: Runtime.NODEJS_22_X,
      architecture: Architecture.ARM_64,
      entry: path.join(
        __dirname,
        '..',
        '..',
        'handlers',
        'scheduler',
        'src',
        'scheduler.mts',
      ),
      handler: 'handler',
      timeout: Duration.seconds(30),
      memorySize: 128,
      logGroup: logGroup,
      environment: {
        TAG_PREFIX: props.tagPrefix,
      },
    });
    const secretTagEventRules = new Rule(this, 'SecretTagEventRules', {
      eventPattern: {
        source: ['aws.secretsmanager'],
        detailType: ['AWS API Call via CloudTrail'],
        detail: {
          eventSource: ['secretsmanager.amazonaws.com'],
          eventName: ['TagResource', 'UntagResource'],
        },
      }
    });
    secretTagEventRules.addTarget(new LambdaFunction(this));
    const dailyRule = new Rule(this, 'DailySchedulerRule', {
      schedule: Schedule.cron({
        minute: '50',
        hour: '23',
        month: '*',
        weekDay: '*',
        year: '*',
      }),
    });
    dailyRule.addTarget(new LambdaFunction(this));
    this.addToRolePolicy(
      new PolicyStatement({
        actions: [
          'states:StartExecution',
          'states:StopExecution',
          'states:ListExecutions',
          'secretsmanager:GetSecretValue',
        ],
        resources: ['*'],
      }),
    );
  }
}

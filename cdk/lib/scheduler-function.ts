// CDK construct that defines the scheduler Lambda responsible for
// reacting to Secrets Manager tag/untag events and a daily cron event.
// It also grants the Lambda minimal permissions to interact with
// Step Functions and Secrets Manager.
import {NodejsFunction} from 'aws-cdk-lib/aws-lambda-nodejs';
import {Construct} from 'constructs';
import {Architecture, Runtime} from 'aws-cdk-lib/aws-lambda';
import {Duration, RemovalPolicy} from 'aws-cdk-lib';
import * as path from 'path';
import {PolicyStatement} from 'aws-cdk-lib/aws-iam';
import {Rule, Schedule} from 'aws-cdk-lib/aws-events';
import {LambdaFunction} from 'aws-cdk-lib/aws-events-targets';
import {LogGroup, RetentionDays} from 'aws-cdk-lib/aws-logs';

/**
 * Props for SchedulerFunction
 * - tagPrefix: Prefix used to detect relevant tags on Secrets Manager secrets.
 */
interface SchedulerFunctionProps {
  readonly tagPrefix: string;
}

export class SchedulerFunction extends NodejsFunction {
  constructor(scope: Construct, id: string, props: SchedulerFunctionProps) {
    // Create a dedicated log group with a short retention for cost control.
    const logGroup = new LogGroup(scope, `${id}LogGroup`, {
      retention: RetentionDays.ONE_WEEK,
      removalPolicy: RemovalPolicy.DESTROY,
    });

    // Define the Node.js Lambda bundling the TypeScript entrypoint.
    super(scope, id, {
      runtime: Runtime.NODEJS_22_X,
      architecture: Architecture.ARM_64,
      entry: path.join(
        __dirname,
        '..',
        '..',
        'handlers',
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

    // EventBridge rule for Secrets Manager Tag/Untag events via CloudTrail.
    const secretTagEventRules = new Rule(this, 'SecretTagEventRules', {
      eventPattern: {
        source: ['aws.secretsmanager'],
        detailType: ['AWS API Call via CloudTrail'],
        detail: {
          eventSource: ['secretsmanager.amazonaws.com'],
          eventName: [
            'TagResource',
            'UntagResource',
            'DeleteSecret',
            'CreateSecret',
          ],
        },
      },
    });
    secretTagEventRules.addTarget(new LambdaFunction(this));

    // Daily scheduled trigger (currently no-op placeholder in the handler). Scan all secrets once a day.
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

    // Grant the Lambda permissions to interact with Step Functions and Secrets Manager.
    this.addToRolePolicy(
      new PolicyStatement({
        actions: [
          'states:StartExecution',
          'states:StopExecution',
          'states:ListExecutions',
          'secretsmanager:GetSecretValue',
          'secretsmanager:DescribeSecret',
          'secretsmanager:ListSecrets',
        ],
        resources: ['*'],
      }),
    );
  }
}

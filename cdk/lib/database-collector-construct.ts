import {Construct} from 'constructs';
import {Duration} from 'aws-cdk-lib';
import {IEventBus} from 'aws-cdk-lib/aws-events';
import {SchedulerFunction} from './scheduler-function';
import {CollectorFunction} from './collector-function';
import {
  Choice,
  Condition,
  JsonPath,
  Pass,
  StateMachine,
  StateMachineType,
  Wait,
  WaitTime,
  Succeed, TaskInput,
} from 'aws-cdk-lib/aws-stepfunctions';
import {LambdaInvoke} from 'aws-cdk-lib/aws-stepfunctions-tasks';

export interface DatabaseCollectorProps {
  readonly tagPrefix?: string;
  readonly eventBus?: IEventBus;
}

export class DatabaseCollector extends Construct {
  constructor(scope: Construct, id: string, props: DatabaseCollectorProps) {
    super(scope, id);

    const tagPrefix = props.tagPrefix ?? 'database-collector:';

    const schedulerFunction = new SchedulerFunction(this, 'SchedulerFunction', {
      tagPrefix,
    });

    const collectorFunction = new CollectorFunction(this, 'CollectorFunction', {});

    // Optional step to enrich input with execution context (kept for debugging/traceability)
    const addExecutionContext = new Pass(this, 'AddExecutionContext', {
      parameters: {
        'name.$': '$.name',
        'continue.$': '$.continue',
        'Execution.$': '$$.Execution',
        'State.$': '$$.State',
        'StateMachine.$': '$$.StateMachine',
      },
    });

    // Wait for 5 minutes before invoking the Lambda
    const waitFiveMinutes = new Wait(this, 'WaitFiveMinutes', {
      time: WaitTime.duration(Duration.seconds(300)),
    });

    // Lambda task that invokes the Java collector with { name }
    const runCollector = new LambdaInvoke(this, 'RunCollector', {
      lambdaFunction: collectorFunction,
      payload: TaskInput.fromObject({
        name: JsonPath.stringAt('$.name'),
      }),
      resultPath: JsonPath.DISCARD, // don't accumulate results to keep the input small in a loop
    });

    const done = new Succeed(this, 'Done');

    // Loop: Wait 5 minutes -> Invoke Lambda -> If input.continue == true then loop else Done
    const shouldContinue = new Choice(this, 'ShouldContinue?')
      .when(Condition.booleanEquals('$.continue', true), waitFiveMinutes)
      .otherwise(done);

    // Chain the states together
    const definition = addExecutionContext
      .next(waitFiveMinutes)
      .next(runCollector)
      .next(shouldContinue);

    const stateMachine = new StateMachine(this, 'CollectorStateMachine', {
      definition,
      stateMachineType: StateMachineType.STANDARD,
    });

    // Provide the state machine ARN to the scheduler and grant permissions to start executions
    schedulerFunction.addEnvironment('STATE_MACHINE_ARN', stateMachine.stateMachineArn);
    stateMachine.grantStartExecution(schedulerFunction);
  }
}

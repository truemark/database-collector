import {Construct} from 'constructs';
import {Duration, RemovalPolicy} from 'aws-cdk-lib';
import {IEventBus, Rule} from 'aws-cdk-lib/aws-events';
import {SchedulerFunction} from './scheduler-function';
import {
  Choice,
  Condition,
  Fail,
  JsonPath,
  Pass,
  StateMachine,
  StateMachineType,
  Wait,
  WaitTime,
  Map as SfnMap,
  DefinitionBody,
} from 'aws-cdk-lib/aws-stepfunctions';

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

    const addExecutionContext = new Pass(this, 'AddExecutionContext', {
      parameters: {
        'Execution.$': '$$.Execution',
        'State.$': '$$.State',
        'StateMachine.$': '$$.StateMachine',
      },
    });

    const wait = new Wait(this, 'WaitForAction', {
      time: WaitTime.timestampPath(JsonPath.stringAt('$.Execution.Input.when')),
    });
  }}

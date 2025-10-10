# Database Collector

Event-driven AWS solution that starts and maintains Step Functions execution loops for AWS Secrets Manager secrets tagged with a configurable prefix. Tagging a secret with a matching key starts a loop that invokes a collector Lambda every 5 minutes; removing or disabling the tag stops it. A daily scheduled scan also reconciles secrets that opt-in via a special key.

## Overview

Components:
- Scheduler Lambda (TypeScript):
  - Listens to Secrets Manager CreateSecret/TagResource/UntagResource/DeleteSecret events via EventBridge (CloudTrail).
  - Starts a Step Functions execution for each matching tag on a secret when the tag value is truthy (true/1/yes/y).
  - Stops the matching execution(s) when the tag is removed or when the tag value is false-y (false/0/no/n).
  - Daily cron: scans all secrets with `<TAG_PREFIX>enabled=true`, stops any RUNNING executions for those secrets/identifiers, and starts a fresh execution.
- Step Functions State Machine:
  - Wait 5 minutes → Invoke Collector Lambda → Check `continue` flag → Loop or Finish.
- Collector Lambda (Java):
  - Example handler that logs input and returns a greeting. Replace with real collection logic.

Tagging model:
- Use a tag key with the configured prefix to start/stop a loop per key. For example: `database-collector:orders`.
- Execution name format: `<secretName>--<identifier>--<random>` to ensure uniqueness while allowing targeted stops.
- The identifier is the tag key suffix after the prefix; if empty, `default` is used.
- Removing the tag key or setting its value to a false-y value stops the corresponding loop.
- Daily scan only considers the special key `<TAG_PREFIX>enabled` and treats any truthy value as enabled.

## Architecture

- EventBridge rule (source: `aws.secretsmanager`, detail type: `AWS API Call via CloudTrail`, event names: `CreateSecret`, `TagResource`, `UntagResource`, `DeleteSecret`) targets the Scheduler Lambda.
- Scheduler starts/stops Step Functions executions for the state machine based on tag events and performs a daily reconciliation scan.
- A daily EventBridge cron rule triggers the Scheduler Lambda to perform the scan that stops any RUNNING execution and starts a new one for secrets with `<TAG_PREFIX>enabled=true`.
- The state machine calls the Collector Lambda every 5 minutes while `continue = true`.

Additional event handling details:
- CreateSecret: Scheduler fetches tags via DescribeSecret and starts executions for any matching truthy tags.
- UntagResource or false-y tag value: Scheduler stops RUNNING executions for the derived identifier.
- DeleteSecret: Scheduler stops any RUNNING executions whose names include the deleted secret’s name fragment.

## Repository Layout

- `cdk/` — CDK app and constructs
  - `lib/database-collector-construct.ts` — High-level construct wiring everything together
  - `lib/database-collector-stack.ts` — Stack that instantiates the construct and outputs metadata
  - `lib/scheduler-function.ts` — CDK for the Scheduler Lambda and EventBridge rules
  - `lib/collector-function.ts` — CDK for packaging the Java collector Lambda
- `collector/` — Java collector Lambda example
  - `src/main/java/com/truemark/collector/HelloWorldHandler.java`
- `handlers/` — TypeScript scheduler Lambda sources
  - `src/scheduler.mts`

## Deploying

Prerequisites:
- Node.js 18+ and npm/pnpm
- AWS CDK v2 installed globally (`npm i -g aws-cdk`) or use npx
- AWS credentials configured with permissions to create Lambda, Step Functions, EventBridge, CloudWatch Logs
- Java 17+ and Maven if you plan to modify/build the Java collector

Install dependencies and deploy:
- Change directory to `cdk`
- Install dependencies: `npm install` or `pnpm install`
- Synthesize: `npx cdk synth`
- Deploy: `npx cdk deploy`

During deploy, the stack will:
- Build and deploy the Scheduler Lambda from `handlers/src/scheduler.mts`
- Build and deploy the Collector Lambda (Java) from `collector/`
- Create the Step Functions state machine and EventBridge rules (including a daily cron)
- Wire environment variables and IAM permissions

## Configuration

Environment variables on the Scheduler Lambda:
- `TAG_PREFIX` (default: `database-collector:`) — Tag key prefix used by the scheduler to detect relevant tags.
- `STATE_MACHINE_ARN` — Injected by the CDK construct; no manual action required.

Truthiness rules for tag values:
- Treated as true: `true`, `1`, `yes`, `y` (case-insensitive, surrounding whitespace ignored)
- Treated as false: anything else or missing value

Daily scan key:
- Special key: `<TAG_PREFIX>enabled`
- Behavior: once per day, the scheduler scans for secrets with this key set to a truthy value, stops any RUNNING executions for that secret/identifier, and starts a new execution with input `{ name, continue: true, secretArn, secretValue, tagKey, tagValue }`.

Behavioral notes:
- On tag events, the scheduler reads the secret value and passes it to the state machine input. Binary secrets are base64-encoded.
- The state machine waits 5 minutes between collector invocations while `continue = true`.

## Using the System

Start a loop via tags:
1. Deploy the stack.
2. Tag a Secrets Manager secret with a key that starts with the configured prefix. For example:
   - Key: `database-collector:orders` (value: `true`)
3. The scheduler Lambda will start a state machine execution named like `<secretName>--orders--<random>`.

Stop a loop:
- Remove the tag key (`database-collector:orders`) from the secret, or
- Set the tag value to a false-y value (e.g., `false` or `0`). The scheduler will stop matching RUNNING executions.

Daily rescan:
- Add the special key `database-collector:enabled=true` to secrets that should be periodically rescheduled.
- Each day, any RUNNING execution for that secret/identifier is stopped and a fresh execution is started.

Multiple loops per secret:
- You can tag the same secret with multiple keys using the prefix to run multiple independent loops (one per identifier).

## Local Development

- Scheduler Lambda (TypeScript): located at `handlers/src/scheduler.mts`, uses AWS SDK v3 clients.
- Collector Lambda (Java): simple example under `collector/`. To rebuild locally, run `mvn -f collector/pom.xml package` (CDK bundling handles this during deploy as well).

## Security and Permissions

- The Scheduler Lambda is granted the following permissions:
  - `states:StartExecution`, `states:StopExecution`, `states:ListExecutions`
  - `secretsmanager:GetSecretValue`, `secretsmanager:DescribeSecret`, `secretsmanager:ListSecrets`
- EventBridge rules invoke the Scheduler Lambda on relevant Secrets Manager events and on a daily schedule.

## Troubleshooting

- No executions start when tagging a secret:
  - Ensure CloudTrail events for Secrets Manager are available (the EventBridge rule listens for CloudTrail API calls).
  - Confirm the tag key starts with the configured `TAG_PREFIX` and the value is truthy.
  - Check CloudWatch Logs for the Scheduler Lambda for errors.
- An execution doesn’t stop after removing or disabling a tag:
  - Verify the exact key you removed/updated had the correct prefix and identifier.
  - Check that a RUNNING execution with the derived name exists.
- Daily rescan did not start a new execution:
  - Confirm the secret has `<TAG_PREFIX>enabled` with a truthy value.
  - Review Scheduler logs for stop/start operations around the cron time.

## Costs

- Step Functions (Standard), Lambda invocations, and CloudWatch Logs incur charges. The state machine runs every 5 minutes while active, and a daily scan lists Secrets Manager secrets.

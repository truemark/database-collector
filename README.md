# Database Collector

Event-driven AWS solution that periodically runs a collector Lambda for Secrets Manager secrets tagged with a specific prefix. Tagging a secret with the configured prefix starts a Step Functions execution loop (every 5 minutes by default) that invokes the collector. Removing the tag stops the loop.

## Overview

Components:
- Scheduler Lambda (TypeScript):
  - Listens to Secrets Manager TagResource/UntagResource events via EventBridge (CloudTrail).
  - Starts a Step Functions execution for each matching tag on a secret.
  - Stops the matching execution(s) when the tag is removed.
  - Also receives a daily scheduled trigger (currently a no-op placeholder).
- Step Functions State Machine:
  - Wait 5 minutes → Invoke Collector Lambda → Check "continue" flag → Loop or Finish.
- Collector Lambda (Java):
  - Example handler that logs input and returns a greeting. Replace with real collection logic.

Tagging model:
- Use a tag key with the configured prefix to start a loop per key. e.g. `database-collector:orders`
- Execution name format: `<secretName>--<identifier>`
- The identifier is the tag key suffix after the prefix; if empty, `default` is used.
- Removing the tag key stops the corresponding loop.

## Architecture

- EventBridge rule (source: `aws.secretsmanager`, detail type: `AWS API Call via CloudTrail`, event names: `TagResource`, `UntagResource`) targets the Scheduler Lambda.
- Scheduler starts/stops Step Functions executions for the state machine.
- A daily EventBridge cron rule also triggers the Scheduler Lambda (placeholder).
- The state machine calls the Collector Lambda every 5 minutes while `continue = true`.

## Repository Layout

- `cdk/` — CDK app and constructs
  - `lib/database-collector-construct.ts` — High-level construct wiring everything together
  - `lib/scheduler-function.ts` — CDK for the Scheduler Lambda and EventBridge rules
- `handlers/collector/` — Java collector Lambda example
  - `src/main/java/com/truemark/collector/HelloWorldHandler.java`
- `handlers/scheduler/` — TypeScript scheduler Lambda
  - `src/scheduler.mts`

## Deploying

Prerequisites:
- Node.js 18+ and npm/pnpm
- AWS CDK v2 installed globally (`npm i -g aws-cdk`) or use npx
- AWS credentials configured with permissions to create Lambda, Step Functions, EventBridge, CloudWatch Logs
- Java 17+ and Maven if you plan to modify/build the Java collector

Install dependencies and bootstrap CDK (if not already bootstrapped in the account/region):

- Change directory to `cdk`
- Install dependencies: `npm install` or `pnpm install`
- Synthesize: `npx cdk synth`
- Deploy: `npx cdk deploy`

During deploy, the stack will:
- Build and deploy the Scheduler Lambda from `handlers/scheduler/src/scheduler.mts`
- Build and deploy the Collector Lambda (Java) from `handlers/collector`
- Create the Step Functions state machine and EventBridge rules
- Wire environment variables and IAM permissions

## Configuration

Environment variables:
- `TAG_PREFIX` (default: `database-collector:`) — Tag key prefix used by the scheduler to detect relevant tags.
- `STATE_MACHINE_ARN` — Injected by the CDK construct into the scheduler Lambda; no action required by users of the construct.

Behavioral notes:
- The scheduler reads the secret value and passes it to the state machine input when a tag event occurs. Binary secrets are base64-encoded.
- The state machine includes `{ name, continue, secretArn, secretValue, tagKey, tagValue }` in input when starting.
- The state machine waits 5 minutes between collector invocations while `continue = true`.

## Using the System

1. Deploy the stack.
2. Tag a Secrets Manager secret with a key that starts with the configured prefix. For example:
   - Key: `database-collector:orders` (value optional)
3. The scheduler Lambda will start a state machine execution named like `<secretName>--orders`.
4. To stop the loop, remove the tag key (`database-collector:orders`) from the secret.

Multiple loops per secret:
- You can tag the same secret with multiple keys using the prefix to run multiple independent loops (one per identifier).

## Local Development

- Scheduler Lambda (TypeScript): minimally typed and uses AWS SDK v3 clients; see inline comments for details.
- Collector Lambda (Java): simple example under `handlers/collector`. To rebuild, run `mvn -f handlers/collector/pom.xml package` (CDK typically handles bundling).

## Security and Permissions

- The Scheduler Lambda is granted the following permissions:
  - `states:StartExecution`, `states:StopExecution`, `states:ListExecutions` on the state machine
  - `secretsmanager:GetSecretValue` for reading secret contents
- EventBridge rules invoke the Scheduler Lambda on relevant events and on a daily schedule.

## Troubleshooting

- No executions start when tagging a secret:
  - Ensure CloudTrail data events for Secrets Manager are available (the EventBridge rule listens for CloudTrail API calls)
  - Confirm the tag key starts with the configured `TAG_PREFIX`
  - Check CloudWatch Logs for the Scheduler Lambda for errors
- An execution doesn’t stop after removing a tag:
  - Verify the exact key you removed had the correct prefix
  - Check that an execution with the derived name exists and is RUNNING

## Costs

- Step Functions (Standard), Lambda invocations, and CloudWatch Logs incur charges. The state machine runs every 5 minutes while active.

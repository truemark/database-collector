/*
Triggers on EventBridge TagResource and UntagResource events for Secrets Manager.
- On TagResource: start a Step Functions execution per matching tag key (prefix match),
  execution name derived from the secret id and tag identifier. Input includes the
  tag identifier, the secret ARN, and the secret value. Also sets continue=true so
  the state machine loops at 5-minute intervals.
- On UntagResource: stop any RUNNING execution(s) matching the derived execution name(s).

Exec name format: <secretName>--<identifier> (sanitized to Step Functions name rules)

Also supports daily scheduled trigger (currently no-op placeholder).
*/

import { SFNClient, StartExecutionCommand, ListExecutionsCommand, StopExecutionCommand } from '@aws-sdk/client-sfn';
import { SecretsManagerClient, GetSecretValueCommand } from '@aws-sdk/client-secrets-manager';

const sfn = new SFNClient({});
const secrets = new SecretsManagerClient({});

function arnToSecretName(arn: string): string {
  // arn:aws:secretsmanager:region:acct:secret:secretName-xxxx
  try {
    const parts = arn.split(':');
    const last = parts[parts.length - 1] || '';
    const afterSecret = last.split('secret:')[1] ?? last; // handle if includes 'secret:'
    // strip random suffix after '-' if present to keep it shorter and deterministic enough
    const base = afterSecret.split('-')[0];
    return base || afterSecret || arn;
  } catch {
    return arn;
  }
}

function sanitizeExecName(name: string): string {
  // Allowed: [.-_A-Za-z0-9], max 80 chars
  const cleaned = name.replace(/[^A-Za-z0-9._-]/g, '-');
  return cleaned.substring(0, 80);
}

function identifierFromTagKey(key: string, prefix: string): string {
  return key.startsWith(prefix) ? key.substring(prefix.length) || 'default' : key;
}

async function getSecretValueString(secretArn: string): Promise<string | undefined> {
  try {
    const res = await secrets.send(new GetSecretValueCommand({ SecretId: secretArn }));
    if (res.SecretString != null) return res.SecretString;
    if (res.SecretBinary != null) return Buffer.from(res.SecretBinary as any).toString('base64');
  } catch (e) {
    console.warn('Failed to get secret value', e);
  }
  return undefined;
}

async function listRunningExecNames(stateMachineArn: string): Promise<Set<string>> {
  const names = new Set<string>();
  let nextToken: string | undefined = undefined;
  do {
    const out = await sfn.send(new ListExecutionsCommand({
      stateMachineArn,
      statusFilter: 'RUNNING',
      nextToken,
    }));
    for (const exec of out.executions ?? []) {
      if (exec.name) names.add(exec.name);
    }
    nextToken = out.nextToken;
  } while (nextToken);
  return names;
}

export async function handler(event: any): Promise<void> {
  console.log('Received event:', JSON.stringify(event, null, 2));
  const tagPrefix  = process.env.TAG_PREFIX || 'database-collector:';
  const stateMachineArn = process.env.STATE_MACHINE_ARN;

  if (!stateMachineArn) {
    console.error('STATE_MACHINE_ARN env var is required');
    return;
  }

  if (
    event &&
    event['detail-type'] === 'AWS API Call via CloudTrail' &&
    (event.detail?.eventName === 'TagResource' || event.detail?.eventName === 'UntagResource')
  ) {
    const secretArn: string | undefined = event.detail?.requestParameters?.secretId;

    if (!secretArn) {
      console.log('No resourceArn found in event; ignoring.');
      return;
    }

    const secretName = arnToSecretName(secretArn);

    if (event.detail?.eventName === 'TagResource') {
      const tags: Array<{ key: string; value?: string }> = event.detail?.requestParameters?.tags || [];
      const matching = tags.filter(t => t?.key?.startsWith(tagPrefix));
      if (matching.length === 0) {
        console.log('TagResource event without matching prefix; ignoring.');
        return;
      }

      // cache running names to avoid duplicate starts
      const runningNames = await listRunningExecNames(stateMachineArn);

      const secretValue = await getSecretValueString(secretArn);

      for (const t of matching) {
        const identifier = identifierFromTagKey(t.key, tagPrefix);
        const execName = sanitizeExecName(`${secretName}--${identifier}`);
        if (runningNames.has(execName)) {
          console.log(`Execution already running for ${execName}; skipping start.`);
          continue;
        }
        const input = {
          name: identifier || secretName,
          continue: true,
          secretArn,
          secretValue,
          tagKey: t.key,
          tagValue: t.value,
        };
        try {
          await sfn.send(new StartExecutionCommand({
            stateMachineArn,
            name: execName,
            input: JSON.stringify(input),
          }));
          console.log(`Started execution ${execName}`);
        } catch (e) {
          console.warn(`Failed to start execution ${execName}:`, e);
        }
      }
    } else if (event.detail?.eventName === 'UntagResource') {
      const tagKeys: string[] = event.detail?.requestParameters?.tagKeys || [];
      const matchingKeys = tagKeys.filter((k: string) => typeof k === 'string' && k.startsWith(tagPrefix));
      if (matchingKeys.length === 0) {
        console.log('UntagResource event without matching prefix; ignoring.');
        return;
      }
      const runningNames = await listRunningExecNames(stateMachineArn);
      for (const key of matchingKeys) {
        const identifier = identifierFromTagKey(key, tagPrefix);
        const execName = sanitizeExecName(`${secretName}--${identifier}`);
        if (!runningNames.has(execName)) {
          console.log(`No running execution found for ${execName}; nothing to stop.`);
          continue;
        }
        // We must find the executionArn to stop; list again and find the one with this name.
        // To avoid an extra full list per key, we can list once here.
        let nextToken: string | undefined = undefined;
        let stopped = false;
        do {
          const page = await sfn.send(new ListExecutionsCommand({ stateMachineArn, statusFilter: 'RUNNING', nextToken }));
          for (const exec of page.executions ?? []) {
            if (exec.name === execName && exec.executionArn) {
              try {
                await sfn.send(new StopExecutionCommand({ executionArn: exec.executionArn, cause: 'Untagged' }));
                console.log(`Stopped execution ${execName}`);
                stopped = true;
                break;
              } catch (e) {
                console.warn(`Failed to stop execution ${execName}:`, e);
              }
            }
          }
          nextToken = page.nextToken;
        } while (!stopped && nextToken);
      }
    }
  } else if (event && event.source === 'aws.events') {
    console.log('Triggered by scheduled (cron) event');
  } else {
    console.log('Unknown event type');
  }
}

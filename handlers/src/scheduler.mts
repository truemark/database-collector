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

import {
  SFNClient,
  StartExecutionCommand,
  ListExecutionsCommand,
  StopExecutionCommand,
  ListExecutionsCommandOutput,
} from '@aws-sdk/client-sfn';
import {
  SecretsManagerClient,
  GetSecretValueCommand,
  DescribeSecretCommand,
  ListSecretsCommand,
} from '@aws-sdk/client-secrets-manager';

// Reusable AWS SDK clients (kept outside the handler for connection reuse across invocations)
const sfn = new SFNClient({});
const secrets = new SecretsManagerClient({});

/**
 * Convert a Secrets Manager ARN into a short, deterministic secret name suitable for Step Functions execution names.
 * Example input: arn:aws:secretsmanager:region:acct:secret:secretName-xxxx
 */
function arnToSecretName(arn: string): string {
  // arn:aws:secretsmanager:region:acct:secret:secretName-xxxx
  try {
    const parts = arn.split(':');
    const last = parts[parts.length - 1] || '';
    const afterSecret = last.split('secret:')[1] ?? last; // handle if includes 'secret:'
    // Use the full secret name (including hyphens) to avoid accidental partial matches
    return afterSecret || arn;
  } catch {
    return arn;
  }
}

/**
 * Sanitize a string for use as a Step Functions execution name.
 * Allowed charset: [.-_A-Za-z0-9], max length: 80 chars.
 */
function sanitizeExecName(name: string): string {
  // Allowed: [.-_A-Za-z0-9], max 80 chars
  const cleaned = name.replace(/[^A-Za-z0-9._-]/g, '-');
  return cleaned.substring(0, 80);
}

/**
 * Extract an identifier from a tag key by removing the configured prefix.
 * If the remainder is empty, returns 'default'.
 */
function identifierFromTagKey(key: string, prefix: string): string {
  return key.startsWith(prefix)
    ? key.substring(prefix.length) || 'default'
    : key;
}

/**
 * Resolve a secret's value as a string.
 * - Returns SecretString directly when present
 * - Encodes SecretBinary as base64
 * - Returns undefined if access fails or no value is present
 */
async function getSecretValueString(
  secretArn: string,
): Promise<string | undefined> {
  try {
    const res = await secrets.send(
      new GetSecretValueCommand({SecretId: secretArn}),
    );
    if (res.SecretString != null) return res.SecretString;
    if (res.SecretBinary != null)
      return Buffer.from(res.SecretBinary as any).toString('base64');
  } catch (e) {
    console.warn('Failed to get secret value', e);
  }
  return undefined;
}

/**
 * List names of currently RUNNING executions for a given state machine.
 */
async function listRunningExecNames(
  stateMachineArn: string,
): Promise<Set<string>> {
  const names = new Set<string>();
  let nextToken: string | undefined = undefined;
  do {
    const out: ListExecutionsCommandOutput = await sfn.send(
      new ListExecutionsCommand({
        stateMachineArn,
        statusFilter: 'RUNNING',
        nextToken,
      }),
    );
    for (const exec of out.executions ?? []) {
      if (exec.name) names.add(exec.name);
    }
    nextToken = out.nextToken;
  } while (nextToken);
  return names;
}

/**
 * Scan all Secrets Manager secrets with the specific enabled tag key and start executions for enabled ones.
 */
async function scanEnabledAndStart(
  stateMachineArn: string,
  tagPrefix: string,
): Promise<void> {
  const enabledTagKey = `${tagPrefix}enabled`;
  const running = await listRunningExecNames(stateMachineArn);
  let nextToken: string | undefined = undefined;
  let examined = 0;
  let started = 0;

  do {
    // @ts-ignore
    const out = await secrets.send(
      new ListSecretsCommand({
        Filters: [{Key: 'tag-key', Values: [enabledTagKey]}],
        NextToken: nextToken,
      }),
    );

    for (const s of out.SecretList ?? []) {
      const secretId = s.ARN || s.Name;
      if (!secretId) continue;
      examined++;
      // Describe to get tags/values reliably
      const tags = await listTags(secretId);
      const tag = tags.find((t) => t.key === enabledTagKey);
      const enabled = isTruthy(tag?.value);
      if (!enabled) continue;
      const secretName = arnToSecretName(secretId);
      const identifier = 'enabled';
      const base = sanitizeExecName(`${secretName}--${identifier}--`);
      const alreadyRunning = Array.from(running).some((n) => n.includes(base));
      if (alreadyRunning) continue;

      const secretValue = await getSecretValueString(secretId);
      const execName = sanitizeExecName(`${base}${randomSuffix()}`);
      const input = {
        name: identifier || secretName,
        continue: true,
        secretArn: secretId,
        secretValue,
        tagKey: enabledTagKey,
        tagValue: tag?.value,
      };
      try {
        await sfn.send(
          new StartExecutionCommand({
            stateMachineArn,
            name: execName,
            input: JSON.stringify(input),
          }),
        );
        started++;
        console.log(`Started execution ${execName} (daily)`);
      } catch (e) {
        console.warn(`Failed to start execution ${execName} (daily):`, e);
      }
    }

    nextToken = out.NextToken;
  } while (nextToken);

  console.log(
    `Daily scan examined ${examined} secrets with key ${enabledTagKey}; started ${started} executions.`,
  );
}

/**
 * Create a short random suffix for execution names to ensure uniqueness.
 * Uses base36 timestamp and a 6-char random string.
 */
function randomSuffix(): string {
  const ts = Date.now().toString(36);
  const rand = Math.random().toString(36).slice(2, 8);
  return `${ts}-${rand}`;
}

/**
 * Lambda handler that:
 * - On Secrets Manager TagResource: starts a Step Functions execution per matching tag key
 * - On Secrets Manager UntagResource: stops matching RUNNING execution(s)
 * - On scheduled cron event: logs a heartbeat (placeholder)
 */
function isTruthy(val: any): boolean {
  if (val === true) return true;
  if (val == null) return false;
  const s = String(val).trim().toLowerCase();
  return s === 'true' || s === '1' || s === 'yes' || s === 'y';
}

async function listTags(
  secretId: string,
): Promise<Array<{key: string; value?: string}>> {
  try {
    const out = await secrets.send(
      new DescribeSecretCommand({SecretId: secretId}),
    );
    const tags = (out.Tags ?? []) as Array<{Key?: string; Value?: string}>;
    return tags.map((t) => ({key: t.Key ?? '', value: t.Value}));
  } catch (e) {
    console.warn('Failed to list tags for secret', secretId, e);
    return [];
  }
}

export async function handler(event: any): Promise<void> {
  console.log('Received event:', JSON.stringify(event, null, 2));
  const tagPrefix = process.env.TAG_PREFIX || 'database-collector:';
  const stateMachineArn = process.env.STATE_MACHINE_ARN;

  if (!stateMachineArn) {
    console.error('STATE_MACHINE_ARN env var is required');
    return;
  }

  if (
    event &&
    event['detail-type'] === 'AWS API Call via CloudTrail' &&
    (event.detail?.eventName === 'TagResource' ||
      event.detail?.eventName === 'UntagResource' ||
      event.detail?.eventName === 'DeleteSecret' ||
      event.detail?.eventName === 'CreateSecret')
  ) {
    let secretId: string | undefined =
      event.detail?.requestParameters?.secretId;
    if (!secretId && event.detail?.eventName === 'CreateSecret') {
      secretId =
        event.detail?.responseElements?.ARN ||
        event.detail?.requestParameters?.name;
    }

    if (!secretId) {
      console.log('No secret identifier found in event; ignoring.');
      return;
    }

    const secretName = arnToSecretName(secretId);

    if (event.detail?.eventName === 'TagResource') {
      const tags: Array<{key: string; value?: string}> =
        event.detail?.requestParameters?.tags || [];
      const matching = tags.filter((t) => t?.key?.startsWith(tagPrefix));
      if (matching.length === 0) {
        console.log('TagResource event without matching prefix; ignoring.');
        return;
      }

      const secretValue = await getSecretValueString(secretId);

      const running = await listRunningExecNames(stateMachineArn);
      for (const t of matching) {
        const identifier = identifierFromTagKey(t.key, tagPrefix);
        const base = sanitizeExecName(`${secretName}--${identifier}--`);
        const alreadyRunning = Array.from(running).some((n) =>
          n.includes(base),
        );
        const enabled = isTruthy(t.value);
        if (enabled) {
          if (alreadyRunning) {
            console.log(
              `Execution already running for ${base}, skipping start.`,
            );
            continue;
          }
          const execName = sanitizeExecName(`${base}${randomSuffix()}`);
          const input = {
            name: identifier || secretName,
            continue: true,
            secretArn: secretId,
            secretValue,
            tagKey: t.key,
            tagValue: t.value,
          };
          try {
            await sfn.send(
              new StartExecutionCommand({
                stateMachineArn,
                name: execName,
                input: JSON.stringify(input),
              }),
            );
            console.log(`Started execution ${execName}`);
          } catch (e) {
            console.warn(`Failed to start execution ${execName}:`, e);
          }
        } else {
          // If value is false-y, stop matching executions for this identifier
          let nextToken: string | undefined = undefined;
          let stopped = 0;
          do {
            const page: ListExecutionsCommandOutput = await sfn.send(
              new ListExecutionsCommand({
                stateMachineArn,
                statusFilter: 'RUNNING',
                nextToken,
              }),
            );
            for (const exec of page.executions ?? []) {
              if (exec.name && exec.name.includes(base) && exec.executionArn) {
                try {
                  await sfn.send(
                    new StopExecutionCommand({
                      executionArn: exec.executionArn,
                      cause: 'DisabledByTag',
                    }),
                  );
                  console.log(`Stopped execution ${exec.name}`);
                  stopped++;
                } catch (e) {
                  console.warn(`Failed to stop execution ${exec.name}:`, e);
                }
              }
            }
            nextToken = page.nextToken;
          } while (nextToken);
          console.log(
            `Stopped ${stopped} execution(s) for ${base} due to false-y tag value.`,
          );
        }
      }
    } else if (event.detail?.eventName === 'CreateSecret') {
      // CloudTrail CreateSecret event often omits tags; fetch them explicitly.
      const tags = await listTags(secretId);
      const matching = tags.filter((t) => t?.key?.startsWith(tagPrefix));
      if (matching.length === 0) {
        console.log(
          'CreateSecret event has no matching tags (via API); ignoring.',
        );
        return;
      }

      const secretValue = await getSecretValueString(secretId);
      const running = await listRunningExecNames(stateMachineArn);

      for (const t of matching) {
        const identifier = identifierFromTagKey(t.key, tagPrefix);
        const base = sanitizeExecName(`${secretName}--${identifier}--`);
        const alreadyRunning = Array.from(running).some((n) =>
          n.includes(base),
        );
        const enabled = isTruthy(t.value);
        if (!enabled) {
          if (alreadyRunning) {
            // If disabled but something is running for this identifier, stop it.
            let nextToken: string | undefined = undefined;
            do {
              const page: ListExecutionsCommandOutput = await sfn.send(
                new ListExecutionsCommand({
                  stateMachineArn,
                  statusFilter: 'RUNNING',
                  nextToken,
                }),
              );
              for (const exec of page.executions ?? []) {
                if (
                  exec.name &&
                  exec.name.includes(base) &&
                  exec.executionArn
                ) {
                  try {
                    await sfn.send(
                      new StopExecutionCommand({
                        executionArn: exec.executionArn,
                        cause: 'DisabledOnCreate',
                      }),
                    );
                    console.log(
                      `Stopped execution ${exec.name} due to disabled tag on CreateSecret`,
                    );
                  } catch (e) {
                    console.warn(`Failed to stop execution ${exec.name}:`, e);
                  }
                }
              }
              nextToken = page.nextToken;
            } while (nextToken);
          } else {
            console.log(
              `Tag ${t.key} disabled on CreateSecret; no running execution for ${base}`,
            );
          }
          continue;
        }
        if (alreadyRunning) {
          console.log(
            `Execution already running for ${base} on CreateSecret, skipping start.`,
          );
          continue;
        }
        const execName = sanitizeExecName(`${base}${randomSuffix()}`);
        const input = {
          name: identifier || secretName,
          continue: true,
          secretArn: secretId,
          secretValue,
          tagKey: t.key,
          tagValue: t.value,
        };
        try {
          await sfn.send(
            new StartExecutionCommand({
              stateMachineArn,
              name: execName,
              input: JSON.stringify(input),
            }),
          );
          console.log(`Started execution ${execName} (CreateSecret)`);
        } catch (e) {
          console.warn(
            `Failed to start execution ${execName} on CreateSecret:`,
            e,
          );
        }
      }
    } else if (event.detail?.eventName === 'UntagResource') {
      const tagKeys: string[] = event.detail?.requestParameters?.tagKeys || [];
      const matchingKeys = tagKeys.filter((k: string) =>
        k.startsWith(tagPrefix),
      );
      if (matchingKeys.length === 0) {
        console.log('UntagResource event without matching prefix; ignoring.');
        return;
      }
      const identifiers = matchingKeys.map((k) =>
        identifierFromTagKey(k, tagPrefix),
      );
      let nextToken: string | undefined = undefined;
      let stoppedCount = 0;
      do {
        const page: ListExecutionsCommandOutput = await sfn.send(
          new ListExecutionsCommand({
            stateMachineArn,
            statusFilter: 'RUNNING',
            nextToken,
          }),
        );
        for (const exec of page.executions ?? []) {
          const name = exec.name ?? '';
          const shouldStop = identifiers.some((id) =>
            name.includes(sanitizeExecName(`${secretName}--${id}--`)),
          );
          if (shouldStop && exec.executionArn) {
            try {
              await sfn.send(
                new StopExecutionCommand({
                  executionArn: exec.executionArn,
                  cause: 'Untagged',
                }),
              );
              console.log(`Stopped execution ${name}`);
              stoppedCount++;
            } catch (e) {
              console.warn(`Failed to stop execution ${name}:`, e);
            }
          }
        }
        nextToken = page.nextToken;
      } while (nextToken);
      console.log(
        `Total executions stopped for secret ${secretName} identifiers [${identifiers.join(', ')}]: ${stoppedCount}`,
      );
    } else if (event.detail?.eventName === 'DeleteSecret') {
      // Stop all RUNNING executions whose name contains the sanitized secret name for deleted secret
      const secretNameFragment = sanitizeExecName(arnToSecretName(secretId));
      let nextToken: string | undefined = undefined;
      let stoppedCount = 0;
      do {
        const page: ListExecutionsCommandOutput = await sfn.send(
          new ListExecutionsCommand({
            stateMachineArn,
            statusFilter: 'RUNNING',
            nextToken,
          }),
        );
        for (const exec of page.executions ?? []) {
          if (
            exec.name &&
            exec.name.includes(secretNameFragment) &&
            exec.executionArn
          ) {
            try {
              await sfn.send(
                new StopExecutionCommand({
                  executionArn: exec.executionArn,
                  cause: 'SecretDeleted',
                }),
              );
              console.log(`Stopped execution ${exec.name}`);
              stoppedCount++;
            } catch (e) {
              console.warn(`Failed to stop execution ${exec.name}:`, e);
            }
          }
        }
        nextToken = page.nextToken;
      } while (nextToken);
      console.log(
        `Total executions stopped for deleted secret ${secretNameFragment}: ${stoppedCount}`,
      );
    }
  } else if (event && event.source === 'aws.events') {
    console.log('Triggered by scheduled (cron) event');
    await scanEnabledAndStart(stateMachineArn, tagPrefix);
  } else {
    console.log('Unknown event type');
  }
}

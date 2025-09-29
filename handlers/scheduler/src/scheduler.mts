/*
Triggers on event bridge tagResource and UntagResource events for secrets
On tag resource, adds execution for the step function
On Untag resource, removes execution for the step function
Use secretId as the execution names to be able to identify them

Also triggers daily to schedule next executions, because exections are going to run for 24hrs, so before the end of the day we need to schedule the next ones
Wait states in the step function to wait until the time is met
*/

export async function handler(event: any): Promise<void> {
  console.log('Received event:', JSON.stringify(event, null, 2));

  const tagPrefix = process.env.TAG_PREFIX;

  if (
    event &&
    event['detail-type'] === 'AWS API Call via CloudTrail' &&
    (event.detail?.eventName === 'TagResource' || event.detail?.eventName === 'UntagResource')
  ) {
    let tagKeys: string[] = [];
    if (event.detail?.eventName === 'TagResource') {
      tagKeys = event.detail?.requestParameters?.tags?.map((tag: any) => tag.key) || [];
    } else if (event.detail?.eventName === 'UntagResource') {
      tagKeys = event.detail?.requestParameters?.tagKeys || [];
    }
    const hasPrefixedTag = tagKeys.some((key: string) => key.startsWith(tagPrefix));
    if (hasPrefixedTag) {
      console.log(`"${event.detail?.eventName}" event with prefix "${tagPrefix}" detected.`);
      // Add your logic here
    } else {
      console.log(`"${event.detail?.eventName}" event does not match prefix.`);
    }
  } else if (
    event &&
    event.source === 'aws.events'
  ) {
    console.log('Triggered by scheduled (cron) event');
  } else {
    console.log('Unknown event type');
  }
}

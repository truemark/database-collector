/*
Triggers on event bridge tagResource and UntagResource events for secrets
On tag resource, adds execution for the step function
On Untag resource, removes execution for the step function
Use secretId as the execution names to be able to identify them

Also triggers daily to schedule next executions, because exections are going to run for 24hrs, so before the end of the day we need to schedule the next ones
Wait states in the step function to wait until the time is met
*/


export async function handler(event: unknown): Promise<void> {
  console.log('Received event:', JSON.stringify(event, null, 2));

  // Example: Distinguish between tag/untag and cron events
  // Adjust the logic as needed based on your actual event structure
  if (
    typeof event === 'object' &&
    event !== null &&
    'detail-type' in event &&
    (event['detail-type'] === 'AWS API Call via CloudTrail')
  ) {
    console.log('Triggered by tag/untag event for secrets');
  } else if (
    typeof event === 'object' &&
    event !== null &&
    'source' in event &&
    event['source'] === 'aws.events'
  ) {
    console.log('Triggered by scheduled (cron) event');
  } else {
    console.log('Unknown event type');
  }
}

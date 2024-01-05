import {Construct} from 'constructs';
import {ExtendedStack, ExtendedStackProps} from 'truemark-cdk-lib/aws-cdk';
import {DatabaseCollector} from './database-collector';

export class DatabaseCollectorStack extends ExtendedStack {
  constructor(scope: Construct, id: string, props?: ExtendedStackProps) {
    super(scope, id, props);
      new DatabaseCollector(this, "DatabaseCollector", {
    });
  }
}

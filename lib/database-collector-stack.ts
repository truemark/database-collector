import {Construct} from 'constructs';
import {DatabaseCollector} from "./construct"
import {ExtendedStack, ExtendedStackProps} from 'truemark-cdk-lib/aws-cdk';

export class DatabaseCollectorStack extends ExtendedStack {
  constructor(scope: Construct, id: string, props?: ExtendedStackProps) {
    super(scope, id, props);
      new DatabaseCollector(this, "DatabaseCollector", {
    });
  }
}

import {Construct} from 'constructs';
import {DatabaseCollector} from './database-collector-construct';
import * as p from '../../package.json';
import {ExtendedStack, ExtendedStackProps} from 'truemark-cdk-lib/aws-cdk';

export class DatabaseCollectorStack extends ExtendedStack {
  constructor(scope: Construct, id: string, props?: ExtendedStackProps) {
    super(scope, id, props);
    this.outputParameter('Name', 'DatabaseCollector');
    this.outputParameter('Version', p.version);
    new DatabaseCollector(this, 'DatabaseCollector', {});
  }
}

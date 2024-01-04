import * as cdk from 'aws-cdk-lib';
import {Construct} from 'constructs';
import {DatabaseCollector} from "./construct"

export class DatabaseCollectorStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props?: cdk.StackProps) {
    super(scope, id, props);
    new DatabaseCollector(this, "DatabaseCollector", {
    })
  }
}

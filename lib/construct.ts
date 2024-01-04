import { Construct } from "constructs";
import * as path from "path";
import {
  aws_iam as iam,
  aws_events_targets as targets,
  aws_events as events,
} from "aws-cdk-lib";
import * as gobuild from '@aws-cdk/aws-lambda-go-alpha';
import { ExtendedGoFunction } from "/home/d3vb00x/repo/truemark/cdk/aws-lambda";

export interface DatabaseCollectorProps {
}

export class DatabaseCollector extends Construct {
  private IAMRole() {
    const role = new iam.Role(this, "Role", {
      assumedBy: new iam.ServicePrincipal("lambda.amazonaws.com")
    })

    role.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "secretsmanager:DescribeSecret",
          "secretsmanager:ListSecrets"
        ],
        resources: ["*"]
      })
    )
    role.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "secretsmanager:GetSecretValue",
        ],
        resources: ["*"],
        conditions: {
          "StringEquals": { "database-collector:enabled": "true" }
        }
      })
    )
    role.addManagedPolicy({
      managedPolicyArn: "arn:aws:iam::aws:policy/CloudWatchFullAccessV2"
    })
    role.addManagedPolicy({
      managedPolicyArn: "arn:aws:iam::aws:policy/AmazonPrometheusRemoteWriteAccess"
    })

    return role
  }

  private buildAndInstallGoLocal(handler: string, entry: string, version: string){
    const role = this.IAMRole()
    const scheduleRule = new events.Rule(this, 'Rule', {
      schedule: events.Schedule.expression('cron(*/5 * * * ? *)')
    })
    const gofn = new ExtendedGoFunction(this, 'Lambda', {
      entry: path.join(__dirname, "../lambda/"),
      deploymentOptions: {
        createDeployment: false,
      },
      environment: {
        NAME: "test"
      },
      bundling: {
        cgoEnabled: false
      },
      role: role,
    })
    scheduleRule.addTarget(new targets.LambdaFunction(gofn))
    return gofn
  }
  constructor(scope: Construct, id: string, props: DatabaseCollectorProps) {
    super(scope, id);
    this.buildAndInstallGoLocal("database-collector", path.join(__dirname, "../lambda/"), "0.0.1")
    // this.buildAndInstallGOLambda("database-collector", path.join(__dirname, "../lambda/"), "main")
  }
}

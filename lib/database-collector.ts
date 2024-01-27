import { Construct } from "constructs";
import * as path from "path";
import { Runtime } from "aws-cdk-lib/aws-lambda";
import {
  aws_iam as iam,
  aws_events_targets as targets,
  Duration,
  aws_events as events,
  aws_ec2 as ec2
} from "aws-cdk-lib";
import { ExtendedGoFunction } from "truemark-cdk-lib/aws-lambda";

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
          "StringEquals": {
            "aws:ResourceTag/database-collector:enabled": "true"
          }
        }
      })
    )
    role.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "ec2:CreateNetworkInterface",
          "ec2:DescribeNetworkInterfaces",
          "ec2:DeleteNetworkInterface"
        ],
        resources: ["*"]
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

  private buildAndInstallGoLocal(){
    const role = this.IAMRole()
    const scheduleRule = new events.Rule(this, 'Rule', {
      schedule: events.Schedule.expression('cron(*/5 * * * ? *)')
    })
    // const servicesVpc = ec2.Vpc.fromLookup(this, "services-vpc", {
    //   vpcName: "services"
    // })
    // console.log(servicesVpc.privateSubnets)
    const gofn = new ExtendedGoFunction(this, 'Lambda', {
      entry: path.join(__dirname, '..', 'collector'),
      memorySize: 1024,
      timeout:Duration.seconds(300),
      // runtime: Runtime.PROVIDED_AL2,
      deploymentOptions: {
        createDeployment: false,
      },
      environment: {
        EXPORTER_TYPE: "cloudwatch"
      },
      bundling: {
        cgoEnabled: false
      },
      role: role,
    })
    // scheduleRule.addTarget(new targets.LambdaFunction(gofn))
    return gofn
  }
  constructor(scope: Construct, id: string, props: DatabaseCollectorProps) {
    super(scope, id);
    this.buildAndInstallGoLocal()
    // this.buildAndInstallGOLambda("database-collector", path.join(__dirname, "../lambda/"), "main")
  }
}

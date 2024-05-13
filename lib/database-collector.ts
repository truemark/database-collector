import {Construct} from "constructs";
import * as path from "path";
import * as fs from 'fs';
import {
  aws_ec2 as ec2,
  aws_events as events,
  aws_events_targets as targets,
  aws_iam as iam,
  aws_ssm as ssm,
  Duration,
} from "aws-cdk-lib";
import {Runtime} from "aws-cdk-lib/aws-lambda"
import {ExtendedGoFunction} from "truemark-cdk-lib/aws-lambda";


export interface DatabaseCollectorProps {
}

export class DatabaseCollector extends Construct {
  private IAMRole(ssmParameter: ssm.StringParameter) {
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
    role.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "ssm:GetParameter"
        ],
        resources: [ssmParameter.parameterArn]
      })
    )
    //TODO: remove full access
    role.addManagedPolicy({
      managedPolicyArn: "arn:aws:iam::aws:policy/CloudWatchFullAccessV2"
    })
    role.addManagedPolicy({
      managedPolicyArn: "arn:aws:iam::aws:policy/AmazonPrometheusRemoteWriteAccess"
    })

    return role
  }

  private CustomMetricsFile() {
    const customMetricsFile = path.join(__dirname, '..', 'collector', 'custom-metrics.toml');
    const customMetrics = fs.readFileSync(customMetricsFile, 'utf8');
    return new ssm.StringParameter(this, 'CustomMetrics', {
      parameterName: this.node.tryGetContext('ssmParameterPath') || "/lambda/database-collector/custom_metrics.toml",
      stringValue: customMetrics,
    })
  }

  private buildAndInstallGoLocal() {
    const ssmParameter = this.CustomMetricsFile()
    const role = this.IAMRole(ssmParameter)
    const exporterType = this.node.tryGetContext('exporterType')
    const prometheusUrl = this.node.tryGetContext('prometheusUrl')
    const vpcId = this.node.tryGetContext('vpcId')
    const subnetIds = this.node.tryGetContext('subnetIds').split(',');
    const securityGroupIds = this.node.tryGetContext('securityGroupIds').split(',');
    const vpc = ec2.Vpc.fromLookup(this, 'VPC', {
      vpcId: vpcId
    })
    const scheduleRule = new events.Rule(this, 'Rule', {
      schedule: events.Schedule.expression('cron(*/5 * * * ? *)')
    })
    const gofn = new ExtendedGoFunction(this, 'Lambda', {
      entry: path.join(__dirname, '..', 'collector'),
      memorySize: 1024,
      timeout: Duration.seconds(300),
      // runtime: Runtime.PROVIDED_AL2,
      deploymentOptions: {
        createDeployment: false,
      },
      environment: {
        EXPORTER_TYPE: exporterType,
        PROMETHEUS_REMOTE_WRITE_URL: prometheusUrl,
        CUSTOM_METRICS_FILE: ssmParameter.parameterName
      },
      bundling: {
        cgoEnabled: false
      },
      role: role,
      vpc: vpc,
      // securityGroups: securityGroupObjects,
      vpcSubnets: {
        subnets: subnetIds.map((id: string) => ec2.Subnet.fromSubnetId(this, `Subnet${id}`, id)),
      },
      allowPublicSubnet:  true,
      securityGroups: securityGroupIds.map((sgId: string) =>
        ec2.SecurityGroup.fromSecurityGroupId(this, `SG${sgId}`, sgId)
      ),
    })
    const rdsEventRule = new events.Rule(this, 'RDSEventRule', {
      eventPattern: {
        source: ['aws.rds']
      }
    })
    const eventsFn = new ExtendedGoFunction(this, 'EventsLambda', {
      entry: path.join(__dirname, '..', 'events-collector'),
      role: role,
      memorySize: 1024,
      environment: {
        PROMETHEUS_REMOTE_WRITE_URL: prometheusUrl,
      },
      timeout: Duration.seconds(300),
      runtime: Runtime.PROVIDED_AL2023,
      deploymentOptions: {
        createDeployment: false,
      },
    })
    rdsEventRule.addTarget(new targets.LambdaFunction(eventsFn))
    scheduleRule.addTarget(new targets.LambdaFunction(gofn))
    return gofn
  }

  constructor(scope: Construct, id: string, props: DatabaseCollectorProps) {
    super(scope, id);
    this.buildAndInstallGoLocal()
    // this.buildAndInstallGOLambda("database-collector", path.join(__dirname, "../lambda/"), "main")
  }
}

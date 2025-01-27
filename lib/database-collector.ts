// cdk should do all build and deploy part for docker and ecs

import {Construct} from "constructs";
import * as path from "path";
import {SubnetFilter} from "aws-cdk-lib/aws-ec2";
import {
  PolicyStatement,
  ManagedPolicy,
  Role,
  ServicePrincipal
} from "aws-cdk-lib/aws-iam";
import {DockerImageAsset, Platform} from 'aws-cdk-lib/aws-ecr-assets';
import {ContainerImage} from 'aws-cdk-lib/aws-ecs';
import {StandardFargateService, StandardFargateCluster,} from "truemark-cdk-lib/aws-ecs";
import {ExtendedGoFunction} from "truemark-cdk-lib/aws-lambda";
import {Runtime} from "aws-cdk-lib/aws-lambda";
import {Rule} from "aws-cdk-lib/aws-events";
import {LambdaFunction} from "aws-cdk-lib/aws-events-targets";
import {Duration} from "aws-cdk-lib";


export interface DatabaseCollectorProps {
}

export class DatabaseCollector extends Construct {
  private prometheusUrl = this.node.tryGetContext('prometheusUrl')
  private buildAndDeployRDSEventsCollector() {
    const role = new Role(this, "Role", {
      assumedBy: new ServicePrincipal("lambda.amazonaws.com")
    })
    role.addManagedPolicy({managedPolicyArn: "arn:aws:iam::aws:policy/CloudWatchFullAccessV2"})
    role.addManagedPolicy({managedPolicyArn: "arn:aws:iam::aws:policy/AmazonPrometheusRemoteWriteAccess"})
    const rdsEventRule = new Rule(this, 'RDSEventRule', {
      eventPattern: {
        source: ['aws.rds']
      }
    })
    const eventsFn = new ExtendedGoFunction(this, 'EventsLambda', {
      entry: path.join(__dirname, '..', 'collector/cmd/events-collector'),
      memorySize: 1024,
      environment: {
        PROMETHEUS_REMOTE_WRITE_URL: this.prometheusUrl,
      },
      timeout: Duration.seconds(300),
      runtime: Runtime.PROVIDED_AL2023,
      role: role,
      deploymentOptions: {
        createDeployment: false,
      },
    })
    rdsEventRule.addTarget(new LambdaFunction(eventsFn))
  }
  private buildAndDeployECSFargate() {
    const subnetIds = this.node.tryGetContext('subnetIds').split(',');
    const vpcName = this.node.tryGetContext('vpcName') || 'services';


    const asset = new DockerImageAsset(this, 'DatabaseCollectorContainerImage', {
      directory: path.join(__dirname, '..', 'collector'),
      file: path.join('build', 'Dockerfile'),
      platform: Platform.LINUX_ARM64
    })

    const cluster = new StandardFargateCluster(this, "Cluster", {
      vpcName: vpcName
    });
    const service = new StandardFargateService(this, 'Service', {
      image: ContainerImage.fromDockerImageAsset(asset),
      cpu: 1024,
      cluster,
      memoryLimitMiB: 2048,
      desiredCount: 1,
      environment: {
        RUN_MODE: "CRON",
        PROMETHEUS_REMOTE_WRITE_URL: this.prometheusUrl
      },
      vpcSubnets: {
        subnetFilters: [SubnetFilter.byIds(subnetIds)]
      }
    })
    service.taskDefinition.addToTaskRolePolicy(new PolicyStatement({
      actions: [
        "secretsmanager:DescribeSecret",
        "secretsmanager:ListSecrets"
      ],
      resources: ["*"]
    }))
    service.taskDefinition.addToTaskRolePolicy(new PolicyStatement({
      actions: [
        "secretsmanager:GetSecretValue",
      ],
      resources: ["*"],
      conditions: {
        "StringEquals": {
          "aws:ResourceTag/database-collector:enabled": "true"
        }
      }
    }))
    service.taskDefinition.taskRole.addManagedPolicy(ManagedPolicy.fromManagedPolicyArn(
      this,
      'PrometheusRemoteWrite',
      'arn:aws:iam::aws:policy/AmazonPrometheusRemoteWriteAccess'))
  }

  constructor(scope: Construct, id: string, props: DatabaseCollectorProps) {
    super(scope, id);
    this.buildAndDeployECSFargate()
    this.buildAndDeployRDSEventsCollector()
  }
}

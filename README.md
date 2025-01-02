# Database Collector

This is an AWS CDK project that deploys a collector that gathers metrics from databases and publishes them to CloudWatch and Prometheus.

## Languages and Frameworks Used

- TypeScript
- Go
- AWS CDK
- Node.js

## Project Structure

The main logic of the collector is written in Go and is located in the `collector` directory. The AWS infrastructure is defined using AWS CDK in TypeScript, and the main file is `lib/database-collector.ts`.

## Setup

1. Install the dependencies:

```bash
pnpm npm install
```

2. Compile the TypeScript code:

```bash
pnpm run build
```

3. Deploy the stack:

```bash
cdk deploy -c vpcName="" -c subnetIds="" -c securityGroupIds="" -c prometheusUrl=""
```

## Configuration
The collector can be configured using context variables in the AWS CDK app. Here are the available options:  
- `vpcName(optional)`: The name of the VPC where the collector will be deployed (default: services). 
- `subnetIds`: A comma-separated list of subnet IDs where the collector will be deployed.
- `securityGroupIds`: A comma-separated list of security group IDs to attach to the collector.
- `prometheusUrl`: The URL of the Prometheus server where the metrics will be published.

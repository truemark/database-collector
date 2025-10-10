import {Construct} from 'constructs';
import {Duration} from 'aws-cdk-lib';
import {
  Architecture,
  Code,
  Function as LambdaFunction,
  Runtime,
} from 'aws-cdk-lib/aws-lambda';
import * as path from 'path';
import * as fs from 'fs';
import {execSync} from 'child_process';
import * as console from 'node:console';

export interface CollectorFunctionProps {
  readonly memorySize?: number;
  readonly timeout?: Duration;
  readonly environment?: {[key: string]: string};
  readonly architecture?: Architecture;
}

/**
 * Deploys the Java HelloWorld Lambda from handlers/collector using CDK asset bundling.
 *
 * Prefers local Maven/Java if available (no Docker) via CDK local bundling. If the
 * local build fails or Maven is not installed, it falls back to Docker bundling.
 *
 * Handler: com.truemark.collector.HelloWorldHandler::handleRequest
 */
export class CollectorFunction extends LambdaFunction {
  constructor(
    scope: Construct,
    id: string,
    props: CollectorFunctionProps = {},
  ) {
    const projectRoot = path.join(__dirname, '..', '..');
    const collectorDir = path.join(projectRoot, 'collector');

    super(scope, id, {
      runtime: Runtime.JAVA_21,
      architecture: props.architecture ?? Architecture.ARM_64,
      handler: 'com.truemark.collector.HelloWorldHandler::handleRequest',
      memorySize: props.memorySize ?? 256,
      timeout: props.timeout ?? Duration.seconds(30),
      environment: props.environment,
      code: Code.fromAsset(collectorDir, {
        bundling: {
          // Try to use local Maven/Java to build without Docker
          local: {
            tryBundle: (outputDir: string) => {
              try {
                execSync('mvn -q -e -U -B -DskipTests package', {
                  cwd: collectorDir,
                  stdio: 'inherit',
                });
                const jarPath = path.join(
                  collectorDir,
                  'target',
                  'collector.jar',
                );
                const outJar = path.join(outputDir, 'collector.jar');
                fs.copyFileSync(jarPath, outJar);
                return true;
              } catch (err) {
                console.log(err);
                // Returning false lets CDK fall back to Docker bundling
                return false;
              }
            },
          },
          // Docker fallback configuration
          image: Runtime.JAVA_21.bundlingImage,
          command: [
            'bash',
            '-c',
            [
              'set -euo pipefail',
              'mvn -q -e -U -B -DskipTests package',
              'cp target/collector.jar /asset-output/collector.jar',
            ].join(' && '),
          ],
          workingDirectory: '/asset-input',
          outputType: undefined, // default is zipped directory
        },
      }),
    });
  }
}

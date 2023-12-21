#!/usr/bin/env node
import 'source-map-support/register';
import * as cdk from 'aws-cdk-lib';
import { DatabaseCollectorStack } from '../lib/database-collector-stack';

const app = new cdk.App();
new DatabaseCollectorStack(app, 'DatabaseCollector', {});

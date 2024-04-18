#!/usr/bin/env node
import { DatabaseCollectorStack } from '../lib/database-collector-stack';
import {ExtendedApp} from 'truemark-cdk-lib/aws-cdk';

const app = new ExtendedApp();
new DatabaseCollectorStack(app, 'DatabaseCollector', {});

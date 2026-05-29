#!/usr/bin/env node

import { existsSync } from 'node:fs';
import { spawn } from 'node:child_process';

import { installedBinaryPath } from '../lib/install.mjs';

const binaryPath = installedBinaryPath();

if (!existsSync(binaryPath)) {
  console.error('futrixdata-cli binary is not installed. Reinstall @futrixdata/cli to download the release binary.');
  process.exit(1);
}

const child = spawn(binaryPath, process.argv.slice(2), {
  stdio: 'inherit',
});

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 0);
});

child.on('error', (error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});

#!/usr/bin/env node

import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

import { renderHomebrewFormula } from './lib/distribution.mjs';

function parseArgs(argv) {
  const out = {
    repo: 'FutrixDev/FutrixData',
  };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    const next = argv[i + 1];
    switch (arg) {
      case '--version':
        out.version = next;
        i += 1;
        break;
      case '--repo':
        out.repo = next;
        i += 1;
        break;
      case '--darwin-arm64-sha256':
        out.darwinArm64SHA256 = next;
        i += 1;
        break;
      case '--darwin-amd64-sha256':
        out.darwinAmd64SHA256 = next;
        i += 1;
        break;
      case '--output':
        out.output = next;
        i += 1;
        break;
      default:
        throw new Error(`unknown argument: ${arg}`);
    }
  }
  return out;
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const formula = renderHomebrewFormula({
    version: args.version,
    repo: args.repo,
    darwinArm64SHA256: args.darwinArm64SHA256,
    darwinAmd64SHA256: args.darwinAmd64SHA256,
  });
  if (!args.output) {
    process.stdout.write(formula);
    return;
  }
  await mkdir(path.dirname(args.output), { recursive: true });
  await writeFile(args.output, formula, 'utf8');
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});

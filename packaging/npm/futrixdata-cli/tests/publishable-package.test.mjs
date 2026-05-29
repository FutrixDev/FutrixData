import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, chmod, writeFile, readFile, readdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import http from 'node:http';
import { execFile as execFileCallback } from 'node:child_process';
import { promisify } from 'node:util';

const execFile = promisify(execFileCallback);

async function createFakeTarball() {
  const root = await mkdtemp(path.join(tmpdir(), 'futrixdata-cli-npm-pack-fixture-'));
  const payloadDir = path.join(root, 'payload');
  const archivePath = path.join(root, 'futrixdata-cli_darwin_arm64.tar.gz');
  await execFile('mkdir', ['-p', payloadDir]);
  const binaryPath = path.join(payloadDir, 'futrixdata-cli');
  await writeFile(binaryPath, '#!/bin/sh\necho npm-pack-ok\n', 'utf8');
  await chmod(binaryPath, 0o755);
  await execFile('tar', ['-czf', archivePath, '-C', payloadDir, 'futrixdata-cli']);
  return { archivePath };
}

async function serveFile(filePath) {
  const payload = await readFile(filePath);
  const server = http.createServer((req, res) => {
    if (req.url === '/releases/download/v9.9.9/futrixdata-cli_darwin_arm64.tar.gz') {
      res.writeHead(200, { 'Content-Type': 'application/gzip' });
      res.end(payload);
      return;
    }
    res.writeHead(404);
    res.end('not found');
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  return {
    baseURL: `http://127.0.0.1:${address.port}/releases`,
    close: async () => new Promise((resolve, reject) => server.close((err) => (err ? reject(err) : resolve()))),
  };
}

test('packed npm package installs and runs futrixdata-cli without repo-relative files', async () => {
  const { archivePath } = await createFakeTarball();
  const server = await serveFile(archivePath);
  const packageDir = path.join(process.cwd(), 'packaging/npm/futrixdata-cli');
  const packOutputDir = await mkdtemp(path.join(tmpdir(), 'futrixdata-cli-npm-pack-output-'));
  const prefixDir = await mkdtemp(path.join(tmpdir(), 'futrixdata-cli-npm-prefix-'));

  try {
    await execFile('npm', ['pack', '--pack-destination', packOutputDir], {
      cwd: packageDir,
      env: {
        ...process.env,
        npm_config_ignore_scripts: 'true',
      },
    });

    const files = await readdir(packOutputDir);
    const tarballName = files.find((name) => name.endsWith('.tgz'));
    assert.ok(tarballName, 'expected npm pack to produce a tarball');

    await execFile('npm', ['install', '-g', path.join(packOutputDir, tarballName), '--prefix', prefixDir], {
      env: {
        ...process.env,
        FUTRIXDATA_CLI_VERSION: 'v9.9.9',
        FUTRIXDATA_CLI_RELEASE_BASE_URL: server.baseURL,
      },
    });

    const installed = path.join(prefixDir, 'bin', 'futrixdata-cli');
    const result = await execFile(installed, []);
    assert.equal(result.stdout.trim(), 'npm-pack-ok');
  } finally {
    await server.close();
  }
});

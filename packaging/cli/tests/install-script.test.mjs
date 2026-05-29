import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, chmod, writeFile, readFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import http from 'node:http';
import { execFile as execFileCallback } from 'node:child_process';
import { promisify } from 'node:util';

const execFile = promisify(execFileCallback);

async function createFakeTarball() {
  const root = await mkdtemp(path.join(tmpdir(), 'futrixdata-cli-install-script-fixture-'));
  const payloadDir = path.join(root, 'payload');
  const archivePath = path.join(root, 'futrixdata-cli_darwin_arm64.tar.gz');
  await execFile('mkdir', ['-p', payloadDir]);
  const binaryPath = path.join(payloadDir, 'futrixdata-cli');
  await writeFile(binaryPath, '#!/bin/sh\necho install-script-ok\n', 'utf8');
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

test('install script downloads and installs futrixdata-cli into target bin dir', async () => {
  const { archivePath } = await createFakeTarball();
  const server = await serveFile(archivePath);
  const homeDir = await mkdtemp(path.join(tmpdir(), 'futrixdata-cli-install-script-home-'));
  const installDir = path.join(homeDir, 'bin');

  try {
    await execFile('sh', ['scripts/install-futrixdata-cli.sh'], {
      cwd: process.cwd(),
      env: {
        ...process.env,
        HOME: homeDir,
        FUTRIXDATA_CLI_VERSION: 'v9.9.9',
        FUTRIXDATA_CLI_RELEASE_BASE_URL: server.baseURL,
        FUTRIXDATA_CLI_INSTALL_DIR: installDir,
      },
    });

    const installedBinary = path.join(installDir, 'futrixdata-cli');
    assert.ok(existsSync(installedBinary));
    const result = await execFile(installedBinary, []);
    assert.equal(result.stdout.trim(), 'install-script-ok');
  } finally {
    await server.close();
  }
});

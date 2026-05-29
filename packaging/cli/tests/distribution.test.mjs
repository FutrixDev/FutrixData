import test from 'node:test';
import assert from 'node:assert/strict';

import {
  buildReleaseAssetName,
  buildReleaseDownloadURL,
  renderHomebrewFormula,
} from '../lib/distribution.mjs';

test('buildReleaseAssetName uses stable platform-specific archive names', () => {
  assert.equal(buildReleaseAssetName({ platform: 'darwin', arch: 'arm64' }), 'futrixdata-cli_darwin_arm64.tar.gz');
  assert.equal(buildReleaseAssetName({ platform: 'darwin', arch: 'amd64' }), 'futrixdata-cli_darwin_amd64.tar.gz');
  assert.equal(buildReleaseAssetName({ platform: 'linux', arch: 'arm64' }), 'futrixdata-cli_linux_arm64.tar.gz');
  assert.equal(buildReleaseAssetName({ platform: 'windows', arch: 'amd64' }), 'futrixdata-cli_windows_amd64.zip');
});

test('buildReleaseDownloadURL supports latest and pinned versions', () => {
  assert.equal(
    buildReleaseDownloadURL({
      repo: 'FutrixDev/FutrixData',
      platform: 'darwin',
      arch: 'arm64',
    }),
    'https://github.com/FutrixDev/FutrixData/releases/latest/download/futrixdata-cli_darwin_arm64.tar.gz',
  );

  assert.equal(
    buildReleaseDownloadURL({
      repo: 'FutrixDev/FutrixData',
      version: 'v1.2.3',
      platform: 'linux',
      arch: 'amd64',
    }),
    'https://github.com/FutrixDev/FutrixData/releases/download/v1.2.3/futrixdata-cli_linux_amd64.tar.gz',
  );
});

test('renderHomebrewFormula pins darwin intel and arm assets without source build', () => {
  const formula = renderHomebrewFormula({
    version: '1.2.3',
    repo: 'FutrixDev/FutrixData',
    darwinArm64SHA256: 'armsha',
    darwinAmd64SHA256: 'amdsha',
  });

  assert.match(formula, /class FutrixdataCli < Formula/);
  assert.match(formula, /version "1\.2\.3"/);
  assert.match(formula, /futrixdata-cli_darwin_arm64\.tar\.gz/);
  assert.match(formula, /futrixdata-cli_darwin_amd64\.tar\.gz/);
  assert.match(formula, /sha256 "armsha"/);
  assert.match(formula, /sha256 "amdsha"/);
  assert.doesNotMatch(formula, /go build/);
});

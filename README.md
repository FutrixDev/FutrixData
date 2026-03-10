# FutrixData Packaging Repository

This repository only owns CI packaging, code signing, notarization, and release publication for FutrixData.
Application source stays in the source repository and is checked out at an exact commit during each packaging run.

## Event Contract

Primary trigger:

- `repository_dispatch`
- Event type: `futrix-package-request`

Supported manual trigger:

- `workflow_dispatch`

Expected payload fields:

- `source_repository`
- `source_ref`
- `source_sha`
- `version`
- `source_run_url`
- `triggered_by`
- `event_name`
- `release`

## Required Secrets

- `SOURCE_REPO_READ_TOKEN`
  - Token that can read the source repository.
- `MACOS_CERTIFICATE_P12_BASE64`
  - Base64 encoded Developer ID Application `.p12`.
- `MACOS_CERTIFICATE_PASSWORD`
  - Password for the `.p12`.
- `MACOS_SIGNING_IDENTITY`
  - Example: `Developer ID Application: Your Company (TEAMID)`.
- `MACOS_NOTARY_KEY_ID`
  - App Store Connect API key ID.
- `MACOS_NOTARY_ISSUER`
  - App Store Connect issuer ID.
- `MACOS_NOTARY_PRIVATE_KEY_BASE64`
  - Base64 encoded `.p8` private key contents.
- `WINDOWS_CERTIFICATE_PFX_BASE64`
  - Base64 encoded code-signing `.pfx`.
- `WINDOWS_CERTIFICATE_PASSWORD`
  - Password for the `.pfx`.

## Optional Repository Variables

- `PRODUCT_NAME`
  - Default: `FutrixData`
- `MACOS_WAILS_PLATFORM`
  - Default: `darwin/universal`
- `WINDOWS_WAILS_PLATFORM`
  - Default: `windows/amd64`
- `MACOS_BUNDLE_ID`
  - Example: `com.futrixdata.app`
- `WINDOWS_TIMESTAMP_URL`
  - Default: `http://timestamp.digicert.com`

## Release Behavior

- Branch build requests upload workflow artifacts.
- Tag build requests also publish a GitHub Release in this repository.

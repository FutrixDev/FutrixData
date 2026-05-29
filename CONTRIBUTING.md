# Contributing

FutrixData welcomes focused contributions that improve the local AI data
gateway, desktop runtime, datasource support, safety checks, packaging, and
developer experience.

Good contributions include:

- fixing datasource adapters or schema discovery behavior;
- improving risk-rule detection, approval behavior, masking, or auditability;
- adding tests for agent, CLI, IPC, frontend, or packaging flows;
- improving desktop UI clarity without weakening safety controls;
- tightening setup, build, release, or troubleshooting instructions.

Please do not commit:

- database credentials, API keys, access tokens, private logs, customer data, or
  local datasource files;
- generated desktop assets such as `frontend/dist/`, `frontend/wailsjs/`, or
  `build/bin/`;
- local agent skill directories, internal task notes, scratch plans, or runtime
  artifacts;
- changes that bypass risk checks, masking, agent identity, or approval gates
  without an explicit security rationale.

Before opening a pull request, run the relevant checks:

```bash
go test $(go list ./... | grep -v '/frontend/node_modules/')
node --test packaging/cli/tests/*.test.mjs
npm --prefix frontend run build
```

UI changes should also run the relevant Vitest files with
`npm --prefix frontend run test -- <test-file>` and be verified in the Wails
desktop runtime, because the desktop shell, IPC daemon, generated bindings, and
packaged frontend assets are part of the product behavior.

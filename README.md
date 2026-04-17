# FutrixData

**The Data Gateway Your AI Agents Trust**

FutrixData is a desktop app for teams who want to use AI with real databases without handing raw credentials to an LLM. It sits between your AI agents and your data systems, keeps credentials on your machine, masks sensitive fields, and blocks risky operations before they run.

- Website: [futrixdata.com](https://futrixdata.com/)
- Downloads: [Latest Release](https://github.com/FutrixDev/FutrixData/releases/latest)

## Why FutrixData

FutrixData is built for a simple goal: let AI help with real data work, while keeping production access safe.

### Agents get data, not credentials

AI agents connect through MCP or Skill-style integrations and receive query results instead of database passwords, connection strings, or raw tokens.

### Risk checks before execution

Every statement goes through the same safety layer before it runs. Destructive changes can be blocked, expensive queries can be flagged, and schema changes can require explicit confirmation.

### One app for every data source

Use one desktop console to connect databases, browse schemas, run queries, inspect results, and work with an AI assistant in the same place.

## What You Can Do

- Connect MySQL, PostgreSQL, MongoDB, Redis, Elasticsearch, ChromaDB, DynamoDB, Cloudflare D1, and more
- Keep credentials encrypted locally on your machine
- Mask PII and sensitive fields before results reach AI agents
- Use natural language to generate queries and charts
- Review SQL in a visual console with schema browsing and execution history
- Add custom safety rules based on datasource, entity pattern, operation type, row count, or cost

## Built for Safe AI Data Access

FutrixData combines three ideas in one product:

1. **AI Agent Data Gateway**  
   Connect AI tools without exposing secrets.

2. **Risk Control Engine**  
   Analyse queries before execution and stop dangerous operations.

3. **Unified Management Console**  
   Work with many data sources from one polished desktop app.

## Getting Started

1. Download FutrixData for your platform.
2. Connect your databases in the desktop app.
3. Enable MCP or Skill integration for your AI tool.
4. Query with confidence knowing every request passes through the risk engine.

## Available Platforms

- macOS Apple Silicon
- macOS Intel
- Windows 64-bit
- Linux 64-bit

You can always find the latest installers on the [Releases](https://github.com/FutrixDev/FutrixData/releases) page.

## About This Repository

This repository is the **public release and packaging repository** for FutrixData.

It is used to:

- publish installers and release assets
- track release notes and versioned binaries
- store packaging and distribution automation

For the product overview, screenshots, and download experience, visit [futrixdata.com](https://futrixdata.com/).

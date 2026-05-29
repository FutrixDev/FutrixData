package aichat

import "strings"

const mysqlPromptModule = `
Datasource: MySQL
- Datasource routing hint: use search_knowledge first when the answer may depend on MySQL-specific syntax, predicate behavior, quoting, or known operational guidance.
- Use describe_entity when you need live columns, indexes, and exact field names from the active working target. If the current focus misses the target, expand discovery first.
- Write MySQL SQL in arguments.statement (no Markdown fences). Use backticks only when needed; never wrap the whole statement in a single pair of backticks.
`

const postgresqlPromptModule = `
Datasource: PostgreSQL
- Datasource routing hint: use search_knowledge first when the answer may depend on PostgreSQL-specific syntax, predicate behavior, quoting, or known operational guidance.
- Use describe_entity when you need live columns, indexes, and exact field names from the active working target. If the current focus misses the target, expand discovery first.
- Write PostgreSQL SQL in arguments.statement (no Markdown fences). Use double quotes only when needed; never wrap the whole statement in quotes.
`

const mongodbPromptModule = `
Datasource: MongoDB
- Datasource routing hint: use search_knowledge first when the answer may depend on MongoDB operator rules, pipeline constraints, quoting, or driver/shell conventions.
- Use describe_entity when you need live collection fields, indexes, or exact mappings from the active working target. If the current focus misses the target, expand discovery first.
- The statement must be either:
  - Mongo shell (human-facing): db.<collection>.find(...).sort(...).limit(...)
  - FutrixData Mongo statement JSON (tool-facing, preferred):
    {"action":"find","collection":"users","filter":{},"limit":100,"options":{"sort":{"_id":1}}}
- In JSON form, cursor options like sort/projection/skip/hint go under "options".
- For listing first N docs, prefer options.sort({_id: 1}) + limit(N) to keep reads bounded and aligned with the _id index.
`

const redisPromptModule = `
Datasource: Redis
- Datasource routing hint: use search_knowledge for Redis runbooks and safety guidance, and use get_redis_command_docs for exact command syntax/output shape.
- Write raw Redis commands in arguments.statement (no Markdown fences).
- Prefer SCAN over KEYS when the keyspace might be large.
`

const elasticsearchPromptModule = `
Datasource: Elasticsearch
- Datasource routing hint: use search_knowledge first when the answer may depend on Elasticsearch query DSL, mappings, or request-shape conventions.
- Use describe_entity when you need live index mappings or exact field names from the active working target. If the current focus misses the target, expand discovery first.
- Write standard REST request syntax in arguments.statement:
  - First line: METHOD /path
  - Optional JSON body in subsequent lines
- Examples:
  - GET /_cat/indices?v
  - POST /<index>/_search
    {"query":{"match_all":{}},"size":10}
- Keep requests small (size) and prefer filters; when sending a JSON body, prefer POST.
`

const dynamodbPromptModule = `
Datasource: DynamoDB (PartiQL)
- Datasource routing hint: use search_knowledge first when the answer may depend on PartiQL syntax, predicate limits, quoting, or known DynamoDB access-pattern guidance. Expand from the working target first, then type, then all.
- Call describe_entity first when you need live table facts such as Partition Key, Sort Key, GSI, or LSI from the active working target.
- Use PartiQL via execute_statement, e.g. SELECT * FROM "<table>" LIMIT 10, only when live verification is still needed or the user explicitly asks to run it.
- Dialect is partiql, not MySQL SQL. Quote table/index names with double quotes when needed; do not use backticks, MySQL hints, SHOW, DESCRIBE, or EXPLAIN.
- Explain whether the predicate can use the table key or a secondary index; filtering on a non-key attribute usually implies a different access path or a scan.
- Prefer equality on the partition key, add sort-key/GSI predicates when available, and keep LIMIT/pageSize small. If both statement LIMIT and pageSize are present, the smaller effective limit applies.
- Pagination uses pagingToken/NextToken handled by the app UI.
`

func normalizePromptModule(text string) string {
	return strings.TrimSpace(text)
}

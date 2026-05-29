package console

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"futrixdata/platform/internal/commandutil"
	"futrixdata/platform/internal/console/window"
	"futrixdata/platform/internal/datasource"
)

const (
	d1ModeLocal       = "local" // legacy datasource option
	d1ModeCloud       = "cloud" // legacy datasource option
	d1ExecutionDev    = "dev"
	d1ExecutionRemote = "remote"
)

var d1ExplainIndexPattern = regexp.MustCompile(`(?i)\bUSING\s+(?:COVERING\s+)?INDEX\s+([A-Za-z0-9_."` + "`" + `-]+)`)

var (
	d1MigrationCreateTablePattern = regexp.MustCompile(`(?is)^\s*create\s+(?:temp(?:orary)?\s+)?(?:virtual\s+)?table\s+(?:if\s+not\s+exists\s+)?([` + "`" + `"\[]?[A-Za-z0-9_.-]+[` + "`" + `"\]]?)`)
	d1MigrationDropTablePattern   = regexp.MustCompile(`(?is)^\s*drop\s+table\s+(?:if\s+exists\s+)?([` + "`" + `"\[]?[A-Za-z0-9_.-]+[` + "`" + `"\]]?)`)
	d1MigrationAlterTablePattern  = regexp.MustCompile(`(?is)^\s*alter\s+table\s+([` + "`" + `"\[]?[A-Za-z0-9_.-]+[` + "`" + `"\]]?)`)
	d1MigrationRenameTablePattern = regexp.MustCompile(`(?is)^\s*rename\s+table\s+([` + "`" + `"\[]?[A-Za-z0-9_.-]+[` + "`" + `"\]]?)`)
	d1MigrationTruncatePattern    = regexp.MustCompile(`(?is)^\s*truncate\s+table\s+([` + "`" + `"\[]?[A-Za-z0-9_.-]+[` + "`" + `"\]]?)`)
)

type d1StatementResult struct {
	Success bool             `json:"success"`
	Results []map[string]any `json:"results"`
	Meta    map[string]any   `json:"meta"`
	Error   string           `json:"error"`
}

type d1APIError struct {
	Message string `json:"message"`
}

type d1APIEnvelope struct {
	Success bool                `json:"success"`
	Errors  []d1APIError        `json:"errors"`
	Result  []d1StatementResult `json:"result"`
}

type d1PagingToken struct {
	Version      int    `json:"v"`
	DatasourceID string `json:"ds"`
	QueryHash    string `json:"q"`
	Offset       int    `json:"o"`
	PageSize     int    `json:"p"`
}

type D1Adapter struct {
	httpClient           *http.Client
	runCommand           func(ctx context.Context, command []string) ([]byte, error)
	resolveWranglerToken func(ctx context.Context, command []string) (string, error)
	now                  func() time.Time
}

func NewD1Adapter() *D1Adapter {
	adapter := &D1Adapter{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		now:        time.Now,
	}
	adapter.runCommand = func(ctx context.Context, command []string) ([]byte, error) {
		return d1RunCommand(ctx, command)
	}
	adapter.resolveWranglerToken = func(ctx context.Context, command []string) (string, error) {
		return d1ResolveWranglerToken(ctx, command, adapter.runCommand)
	}
	return adapter
}

func (a *D1Adapter) TestConnection(ctx context.Context, ds datasource.DataSource) error {
	_, err := a.Execute(ctx, ds, "SELECT 1", ExecuteOptions{PageSize: 1})
	return err
}

func (a *D1Adapter) ListEntities(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error) {
	statement := d1BuildListEntitiesStatement(opts.Pattern, "", 0)
	rows, _, err := a.queryRows(ctx, ds, statement)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(asString(row["name"]))
		if name == "" || d1IsHiddenSystemEntity(name) {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

func (a *D1Adapter) entityKindsFromRows(rows []map[string]any) map[string]string {
	var kinds map[string]string
	for _, row := range rows {
		name := strings.TrimSpace(asString(row["name"]))
		if name == "" || d1IsHiddenSystemEntity(name) {
			continue
		}
		typ := strings.TrimSpace(asString(row["type"]))
		if strings.EqualFold(typ, "view") {
			if kinds == nil {
				kinds = make(map[string]string)
			}
			kinds[name] = "view"
		}
	}
	return kinds
}

func (a *D1Adapter) ListEntitiesPage(ctx context.Context, ds datasource.DataSource, opts ListOptions, cursor string) (EntityPage, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}
	statement := d1BuildListEntitiesStatement(opts.Pattern, cursor, limit+1)
	rows, _, err := a.queryRows(ctx, ds, statement)
	if err != nil {
		return EntityPage{}, err
	}
	items := make([]string, 0, limit+1)
	for _, row := range rows {
		name := strings.TrimSpace(asString(row["name"]))
		if name == "" || d1IsHiddenSystemEntity(name) {
			continue
		}
		items = append(items, name)
	}
	if len(items) == 0 {
		return EntityPage{Items: nil, Cursor: "", Done: true}, nil
	}
	kinds := a.entityKindsFromRows(rows)
	if len(items) > limit {
		items = items[:limit]
		return EntityPage{Items: items, Cursor: items[len(items)-1], Done: false, Kinds: kinds}, nil
	}
	return EntityPage{Items: items, Cursor: "", Done: true, Kinds: kinds}, nil
}

func (a *D1Adapter) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (DescribeResult, error) {
	table := strings.TrimSpace(name)
	if table == "" {
		return DescribeResult{}, errors.New("table is required")
	}
	lookupName := d1MetadataEntityName(table)
	escapedTable := d1QuoteSQLString(lookupName)
	entityKind := ""
	definitionSQL := ""

	metaRows, _, err := a.queryRows(ctx, ds, fmt.Sprintf("SELECT type, sql FROM sqlite_master WHERE lower(name) = lower('%s') LIMIT 1", escapedTable))
	if err != nil {
		if d1IsSQLiteAuthError(err) && d1IsHiddenSystemEntity(lookupName) {
			metaRows = nil
		} else {
			return DescribeResult{}, err
		}
	}
	if len(metaRows) > 0 {
		entityKind = strings.TrimSpace(asString(metaRows[0]["type"]))
		definitionSQL = strings.TrimSpace(asString(metaRows[0]["sql"]))
	}

	columnRows, _, err := a.queryRows(ctx, ds, fmt.Sprintf("PRAGMA table_info('%s')", escapedTable))
	if err != nil {
		if d1IsSQLiteAuthError(err) && d1IsHiddenSystemEntity(lookupName) {
			columnRows = nil
		} else {
			return DescribeResult{}, err
		}
	}

	type orderedPrimaryColumn struct {
		seq  int
		name string
	}
	columns := make([]ColumnInfo, 0, len(columnRows))
	primaryColumns := make([]orderedPrimaryColumn, 0, 2)
	for _, row := range columnRows {
		colName := strings.TrimSpace(asString(row["name"]))
		if colName == "" {
			continue
		}
		dataType := strings.TrimSpace(asString(row["type"]))
		if dataType == "" {
			dataType = "TEXT"
		}
		nullable := "YES"
		if notNull, ok := toInt64(row["notnull"]); ok && notNull != 0 {
			nullable = "NO"
		}
		col := ColumnInfo{Name: colName, DataType: dataType, Nullable: nullable}
		if rawDefault, ok := row["dflt_value"]; ok && rawDefault != nil {
			col.DefaultValue = rawDefault
		}
		columns = append(columns, col)
		if rawPK, ok := toInt64(row["pk"]); ok && rawPK > 0 {
			primaryColumns = append(primaryColumns, orderedPrimaryColumn{
				seq:  int(rawPK),
				name: colName,
			})
		}
	}

	indexRows, _, err := a.queryRows(ctx, ds, fmt.Sprintf("PRAGMA index_list('%s')", escapedTable))
	if err != nil {
		if d1IsSQLiteAuthError(err) && d1IsHiddenSystemEntity(lookupName) {
			indexRows = nil
		} else {
			return DescribeResult{}, err
		}
	}
	indexes := make([]IndexInfo, 0, len(indexRows))
	for _, row := range indexRows {
		indexName := strings.TrimSpace(asString(row["name"]))
		if indexName == "" {
			continue
		}
		unique := false
		if rawUnique, ok := toInt64(row["unique"]); ok && rawUnique != 0 {
			unique = true
		}
		indexColumns, err := a.d1IndexColumns(ctx, ds, indexName)
		if err != nil {
			if d1IsSQLiteAuthError(err) && d1IsHiddenSystemEntity(table) {
				continue
			}
			return DescribeResult{}, err
		}
		indexes = append(indexes, IndexInfo{
			Name:   indexName,
			Column: strings.Join(indexColumns, ", "),
			Unique: unique,
		})
	}
	hasPrimaryIndex := false
	for _, idx := range indexes {
		if strings.EqualFold(strings.TrimSpace(idx.Name), "primary") {
			hasPrimaryIndex = true
			break
		}
	}
	if len(primaryColumns) > 0 && !hasPrimaryIndex {
		sort.Slice(primaryColumns, func(i, j int) bool { return primaryColumns[i].seq < primaryColumns[j].seq })
		cols := make([]string, 0, len(primaryColumns))
		for _, item := range primaryColumns {
			cols = append(cols, item.name)
		}
		indexes = append(indexes, IndexInfo{
			Name:       "PRIMARY",
			Column:     strings.Join(cols, ", "),
			Unique:     true,
			Definition: "PRIMARY KEY",
		})
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i].Name < indexes[j].Name })

	return DescribeResult{
		Columns:       columns,
		Indexes:       indexes,
		EntityKind:    entityKind,
		DefinitionSQL: definitionSQL,
		Details: []DetailItem{
			{Label: "Engine", Value: "Cloudflare D1 (SQLite)"},
			{Label: "Table", Value: table},
			{Label: "Object Type", Value: entityKind},
		},
	}, nil
}

func d1MetadataEntityName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	parts := d1SplitQualifiedIdentifier(trimmed)
	if len(parts) == 0 {
		return d1UnquoteIdentifierSegment(trimmed)
	}
	return d1UnquoteIdentifierSegment(parts[len(parts)-1])
}

func d1SplitQualifiedIdentifier(name string) []string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	parts := make([]string, 0, 2)
	var current strings.Builder
	var quote byte
	for i := 0; i < len(trimmed); i++ {
		ch := trimmed[i]
		if quote == 0 {
			switch ch {
			case '.':
				part := strings.TrimSpace(current.String())
				if part != "" {
					parts = append(parts, part)
				}
				current.Reset()
				continue
			case '"', '`':
				quote = ch
			case '[':
				quote = ']'
			}
			current.WriteByte(ch)
			continue
		}

		current.WriteByte(ch)
		if ch != quote {
			continue
		}
		if quote != ']' && i+1 < len(trimmed) && trimmed[i+1] == quote {
			current.WriteByte(trimmed[i+1])
			i++
			continue
		}
		quote = 0
	}
	part := strings.TrimSpace(current.String())
	if part != "" {
		parts = append(parts, part)
	}
	return parts
}

func d1UnquoteIdentifierSegment(segment string) string {
	trimmed := strings.TrimSpace(segment)
	if len(trimmed) >= 2 {
		switch {
		case strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\""):
			return strings.ReplaceAll(trimmed[1:len(trimmed)-1], `""`, `"`)
		case strings.HasPrefix(trimmed, "`") && strings.HasSuffix(trimmed, "`"):
			return strings.ReplaceAll(trimmed[1:len(trimmed)-1], "``", "`")
		case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
			return strings.ReplaceAll(trimmed[1:len(trimmed)-1], "]]", "]")
		}
	}
	return trimmed
}

func (a *D1Adapter) d1IndexColumns(ctx context.Context, ds datasource.DataSource, indexName string) ([]string, error) {
	escapedIndex := d1QuoteSQLString(indexName)
	rows, _, err := a.queryRows(ctx, ds, fmt.Sprintf("PRAGMA index_info('%s')", escapedIndex))
	if err != nil {
		return nil, err
	}
	type orderedColumn struct {
		seq  int
		name string
	}
	ordered := make([]orderedColumn, 0, len(rows))
	for _, row := range rows {
		colName := strings.TrimSpace(asString(row["name"]))
		if colName == "" {
			continue
		}
		seq := 0
		if rawSeq, ok := toInt64(row["seqno"]); ok {
			seq = int(rawSeq)
		}
		ordered = append(ordered, orderedColumn{seq: seq, name: colName})
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].seq < ordered[j].seq })
	out := make([]string, 0, len(ordered))
	for _, item := range ordered {
		out = append(out, item.name)
	}
	return out, nil
}

func (a *D1Adapter) Execute(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error) {
	start := time.Now()
	trimmed := strings.TrimSpace(statement)
	if trimmed == "" {
		return QueryResult{}, errors.New("statement required")
	}

	lowered := strings.ToLower(trimmed)
	if d1IsQueryStatement(lowered) {
		return a.executeQuery(ctx, ds, trimmed, opts, start)
	}

	results, elapsedFromMeta, err := a.executeRaw(ctx, ds, trimmed)
	if err != nil {
		return QueryResult{}, err
	}
	if d1ExecutionMode(ds.Options) == d1ExecutionDev {
		if err := a.recordSchemaMigrationIfNeeded(ds, trimmed); err != nil {
			return QueryResult{}, fmt.Errorf("statement executed but failed to record migration: %w", err)
		}
	}
	first := d1FirstStatementResult(results)
	elapsed := elapsedFromMeta
	if elapsed <= 0 {
		elapsed = time.Since(start).Milliseconds()
	}
	rowsAffected := d1RowsWritten(first.Meta)
	if rowsAffected <= 0 {
		rowsAffected = int64(len(first.Results))
	}
	return QueryResult{
		Columns:   d1ColumnsFromRows(first.Results),
		Rows:      first.Results,
		RowCount:  rowsAffected,
		HasMore:   false,
		NextToken: "",
		PrevToken: "",
		ElapsedMs: elapsed,
	}, nil
}

func (a *D1Adapter) executeQuery(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions, start time.Time) (QueryResult, error) {
	info := findTopLevelLimit(statement)
	pageSize := d1ClampPageSize(opts.PageSize)
	offset := 0
	pagingEnabled := !info.found
	statementHash := d1StatementHash(statement)

	if opts.PagingToken != "" {
		if info.found {
			return QueryResult{}, errors.New("paging token is not supported for statements with LIMIT")
		}
		token, err := d1DecodePagingToken(opts.PagingToken)
		if err != nil {
			return QueryResult{}, err
		}
		if token.DatasourceID != "" && token.DatasourceID != ds.ID {
			return QueryResult{}, errors.New("paging token datasource mismatch")
		}
		if token.QueryHash != "" && token.QueryHash != statementHash {
			return QueryResult{}, errors.New("paging token query mismatch")
		}
		if token.PageSize > 0 {
			pageSize = d1ClampPageSize(token.PageSize)
		}
		offset = token.Offset
		if offset < 0 {
			offset = 0
		}
		pagingEnabled = true
	}

	fetchStatement := statement
	if pagingEnabled {
		fetchStatement = d1AppendLimitOffset(statement, pageSize+1, offset)
	}

	results, elapsedFromMeta, err := a.executeRaw(ctx, ds, fetchStatement)
	if err != nil {
		return QueryResult{}, err
	}
	first := d1FirstStatementResult(results)
	rows := first.Results
	columns := d1ColumnsFromRows(rows)
	elapsed := elapsedFromMeta
	if elapsed <= 0 {
		elapsed = time.Since(start).Milliseconds()
	}

	if !pagingEnabled {
		result := QueryResult{
			Columns:      columns,
			Rows:         rows,
			RowCount:     int64(len(rows)),
			HasMore:      false,
			NextToken:    "",
			PrevToken:    "",
			SourceEntity: sqlSourceEntityHint(statement, "sqlite"),
			ElapsedMs:    elapsed,
		}
		if info.found && info.parsed {
			setQueryLimitMetadata(&result, int(info.count), EffectiveLimitStatement)
		} else {
			setQueryLimitMetadata(&result, 0, EffectiveLimitNone)
		}
		return result, nil
	}

	hasMore := len(rows) > pageSize
	if hasMore {
		rows = rows[:pageSize]
	}
	nextToken := ""
	prevToken := ""
	if hasMore {
		next, err := d1EncodePagingToken(d1PagingToken{
			Version:      1,
			DatasourceID: ds.ID,
			QueryHash:    statementHash,
			Offset:       offset + len(rows),
			PageSize:     pageSize,
		})
		if err != nil {
			return QueryResult{}, err
		}
		nextToken = next
	}
	if offset > 0 {
		prevOffset := offset - pageSize
		if prevOffset < 0 {
			prevOffset = 0
		}
		prev, err := d1EncodePagingToken(d1PagingToken{
			Version:      1,
			DatasourceID: ds.ID,
			QueryHash:    statementHash,
			Offset:       prevOffset,
			PageSize:     pageSize,
		})
		if err != nil {
			return QueryResult{}, err
		}
		prevToken = prev
	}

	result := QueryResult{
		Columns:      columns,
		Rows:         rows,
		RowCount:     int64(len(rows)),
		HasMore:      hasMore,
		NextToken:    nextToken,
		PrevToken:    prevToken,
		SourceEntity: sqlSourceEntityHint(statement, "sqlite"),
		ElapsedMs:    elapsed,
	}
	source := EffectiveLimitDefault
	if opts.PagingToken != "" {
		source = EffectiveLimitPagingToken
	} else if opts.PageSize > 0 {
		source = EffectiveLimitPageSize
	}
	setQueryLimitMetadata(&result, pageSize, source)
	return result, nil
}

func (a *D1Adapter) Explain(ctx context.Context, ds datasource.DataSource, statement string) (ExplainResult, error) {
	trimmed := strings.TrimSpace(statement)
	if trimmed == "" {
		return ExplainResult{}, errors.New("statement required")
	}
	explainStatement := trimmed
	if _, hasExplain := cutLeadingKeyword(trimmed, "explain"); !hasExplain {
		if d1HasQueryPlanPrefix(trimmed) {
			explainStatement = "EXPLAIN " + trimmed
		} else {
			explainStatement = "EXPLAIN QUERY PLAN " + trimmed
		}
	}

	rows, _, err := a.queryRows(ctx, ds, explainStatement)
	if err != nil {
		return ExplainResult{}, err
	}
	stages := make([]string, 0, len(rows))
	indexes := make([]string, 0, 2)
	indexSeen := make(map[string]bool)
	usesIndex := false

	for _, row := range rows {
		detail := strings.TrimSpace(asString(row["detail"]))
		if detail == "" {
			continue
		}
		stages = append(stages, detail)
		matches := d1ExplainIndexPattern.FindAllStringSubmatch(detail, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			idx := strings.Trim(match[1], `"'`+"`")
			if idx == "" || indexSeen[idx] {
				continue
			}
			indexSeen[idx] = true
			indexes = append(indexes, idx)
		}
		if d1DetailUsesIndex(detail) {
			usesIndex = true
		}
	}

	return ExplainResult{
		UsesIndex: usesIndex,
		Indexes:   indexes,
		Stages:    stages,
		Detail:    rows,
	}, nil
}

func (a *D1Adapter) ListDatabases(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error) {
	_ = ctx
	candidates := []string{
		strings.TrimSpace(ds.Database),
		strings.TrimSpace(optionString(ds.Options, "databaseName")),
		strings.TrimSpace(optionString(ds.Options, "databaseId")),
	}
	pattern := strings.ToLower(strings.TrimSpace(opts.Pattern))
	out := make([]string, 0, 1)
	seen := make(map[string]bool)
	for _, item := range candidates {
		if item == "" || seen[item] {
			continue
		}
		if pattern != "" && !strings.Contains(strings.ToLower(item), pattern) {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out, nil
}

func (a *D1Adapter) DeployMigrations(ctx context.Context, ds datasource.DataSource) error {
	if !d1DatasourceSupportsDev(ds.Options) {
		return errors.New("dev mode is not supported for this datasource")
	}
	migrationsDir := strings.TrimSpace(optionString(ds.Options, "migrationsDir"))
	if migrationsDir == "" {
		return errors.New("migrationsDir is required for deploy")
	}
	databaseName := strings.TrimSpace(optionString(ds.Options, "databaseName"))
	if databaseName == "" {
		databaseName = strings.TrimSpace(ds.Database)
	}
	if databaseName == "" {
		return errors.New("databaseName is required for deploy")
	}

	command, err := d1WranglerBaseCommand(ds.Options)
	if err != nil {
		return err
	}
	command = append(command, "d1", "migrations", "apply", databaseName, "--remote")
	configPath := strings.TrimSpace(optionString(ds.Options, "wranglerConfigPath"))
	if configPath != "" {
		command = append(command, "--config", configPath)
	}
	_, err = a.runCommand(ctx, command)
	return err
}

func (a *D1Adapter) recordSchemaMigrationIfNeeded(ds datasource.DataSource, statement string) error {
	if !d1DatasourceSupportsDev(ds.Options) {
		return nil
	}
	migrationsDir := strings.TrimSpace(optionString(ds.Options, "migrationsDir"))
	if migrationsDir == "" {
		return nil
	}
	if !d1IsSchemaMigrationStatement(statement) {
		return nil
	}

	configPath := strings.TrimSpace(optionString(ds.Options, "wranglerConfigPath"))
	if configPath == "" {
		return errors.New("wranglerConfigPath is required for dev mode")
	}
	baseDir := filepath.Dir(configPath)
	targetDir := migrationsDir
	if !filepath.IsAbs(targetDir) {
		targetDir = filepath.Join(baseDir, filepath.FromSlash(migrationsDir))
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	filename := d1MigrationFilename(a.now(), statement)
	path := filepath.Join(targetDir, filename)
	seq := 2
	for {
		_, err := os.Stat(path)
		switch {
		case err == nil:
			ext := filepath.Ext(filename)
			base := strings.TrimSuffix(filename, ext)
			path = filepath.Join(targetDir, fmt.Sprintf("%s_%d%s", base, seq, ext))
			seq++
		case errors.Is(err, os.ErrNotExist):
			goto writeFile
		default:
			return err
		}
	}

writeFile:
	content := strings.TrimSpace(statement)
	if content == "" {
		return nil
	}
	if !strings.HasSuffix(content, ";") {
		content += ";"
	}
	content += "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func d1IsSchemaMigrationStatement(statement string) bool {
	trimmed := d1StripLeadingComments(statement)
	if trimmed == "" {
		return false
	}
	return d1MigrationCreateTablePattern.MatchString(trimmed) ||
		d1MigrationDropTablePattern.MatchString(trimmed) ||
		d1MigrationAlterTablePattern.MatchString(trimmed) ||
		d1MigrationRenameTablePattern.MatchString(trimmed) ||
		d1MigrationTruncatePattern.MatchString(trimmed)
}

func d1MigrationFilename(now time.Time, statement string) string {
	prefix := now.UTC().Format("20060102150405")
	return prefix + "_" + d1MigrationLabel(statement) + ".sql"
}

func d1MigrationLabel(statement string) string {
	trimmed := d1StripLeadingComments(statement)
	if trimmed == "" {
		return "schema_change"
	}
	if matches := d1MigrationCreateTablePattern.FindStringSubmatch(trimmed); len(matches) >= 2 {
		return "create_table_" + d1MigrationEntityName(matches[1])
	}
	if matches := d1MigrationDropTablePattern.FindStringSubmatch(trimmed); len(matches) >= 2 {
		return "drop_table_" + d1MigrationEntityName(matches[1])
	}
	if matches := d1MigrationAlterTablePattern.FindStringSubmatch(trimmed); len(matches) >= 2 {
		return "alter_table_" + d1MigrationEntityName(matches[1])
	}
	if matches := d1MigrationRenameTablePattern.FindStringSubmatch(trimmed); len(matches) >= 2 {
		return "rename_table_" + d1MigrationEntityName(matches[1])
	}
	if matches := d1MigrationTruncatePattern.FindStringSubmatch(trimmed); len(matches) >= 2 {
		return "truncate_table_" + d1MigrationEntityName(matches[1])
	}
	return "schema_change"
}

func d1MigrationEntityName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.Trim(trimmed, "`\"[]")
	if idx := strings.LastIndex(trimmed, "."); idx >= 0 {
		trimmed = trimmed[idx+1:]
	}
	trimmed = strings.ToLower(trimmed)
	re := regexp.MustCompile(`[^a-z0-9_]+`)
	normalized := re.ReplaceAllString(trimmed, "_")
	normalized = strings.Trim(normalized, "_")
	if normalized == "" {
		return "entity"
	}
	return normalized
}

func d1StripLeadingComments(statement string) string {
	remaining := strings.TrimLeft(statement, " \t\r\n")
	for remaining != "" {
		switch {
		case strings.HasPrefix(remaining, "--"), strings.HasPrefix(remaining, "#"):
			idx := strings.IndexByte(remaining, '\n')
			if idx < 0 {
				return ""
			}
			remaining = strings.TrimLeft(remaining[idx+1:], " \t\r\n")
		case strings.HasPrefix(remaining, "/*"):
			idx := strings.Index(remaining, "*/")
			if idx < 0 {
				return ""
			}
			remaining = strings.TrimLeft(remaining[idx+2:], " \t\r\n")
		default:
			return remaining
		}
	}
	return ""
}

func (a *D1Adapter) queryRows(ctx context.Context, ds datasource.DataSource, statement string) ([]map[string]any, int64, error) {
	results, elapsed, err := a.executeRaw(ctx, ds, statement)
	if err != nil {
		return nil, 0, err
	}
	first := d1FirstStatementResult(results)
	return first.Results, elapsed, nil
}

func (a *D1Adapter) executeRaw(ctx context.Context, ds datasource.DataSource, statement string) ([]d1StatementResult, int64, error) {
	mode := d1ExecutionMode(ds.Options)
	var err error
	var raw []byte
	switch mode {
	case d1ExecutionDev:
		raw, err = a.executeLocal(ctx, ds, statement)
	case d1ExecutionRemote:
		raw, err = a.executeCloud(ctx, ds, statement)
	default:
		err = fmt.Errorf("unsupported d1 execution mode: %s", mode)
	}
	if err != nil {
		return nil, 0, err
	}
	results, err := d1DecodeStatementResults(raw)
	if err != nil {
		return nil, 0, err
	}
	if len(results) == 0 {
		return nil, 0, nil
	}
	for _, result := range results {
		if !result.Success {
			if msg := strings.TrimSpace(result.Error); msg != "" {
				return nil, 0, errors.New(msg)
			}
			return nil, 0, errors.New("d1 query failed")
		}
	}
	elapsed := d1ElapsedMs(results[0].Meta)
	return results, elapsed, nil
}

func (a *D1Adapter) executeLocal(ctx context.Context, ds datasource.DataSource, statement string) ([]byte, error) {
	command, cleanup, err := d1LocalCommand(ds, statement)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	return a.runCommand(ctx, command)
}

func (a *D1Adapter) executeCloud(ctx context.Context, ds datasource.DataSource, statement string) ([]byte, error) {
	accountID := strings.TrimSpace(optionString(ds.Options, "accountId"))
	if accountID == "" {
		return nil, errors.New("accountId is required for cloud mode")
	}
	databaseID := strings.TrimSpace(optionString(ds.Options, "databaseId"))
	if databaseID == "" {
		return nil, errors.New("databaseId is required for d1")
	}
	token, err := a.cloudToken(ctx, ds)
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimSpace(optionString(ds.Options, "apiBaseURL"))
	if baseURL == "" {
		baseURL = "https://api.cloudflare.com/client/v4"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	endpoint := fmt.Sprintf(
		"%s/accounts/%s/d1/database/%s/query",
		baseURL,
		url.PathEscape(accountID),
		url.PathEscape(databaseID),
	)
	payload := map[string]any{"sql": statement}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}
		if snippet != "" {
			return nil, fmt.Errorf("d1 api request failed: %s: %s", resp.Status, snippet)
		}
		return nil, fmt.Errorf("d1 api request failed: %s", resp.Status)
	}
	return raw, nil
}

func (a *D1Adapter) cloudToken(ctx context.Context, ds datasource.DataSource) (string, error) {
	authMode := strings.ToLower(strings.TrimSpace(optionString(ds.Options, "authMode")))
	if authMode == "" {
		authMode = "wrangler"
	}
	switch authMode {
	case "token":
		token := strings.TrimSpace(optionString(ds.Options, "apiToken"))
		if token == "" {
			return "", errors.New("apiToken is required when authMode=token")
		}
		return token, nil
	case "wrangler":
		command, err := d1WranglerBaseCommand(ds.Options)
		if err != nil {
			return "", err
		}
		token, err := a.resolveWranglerToken(ctx, command)
		if err != nil {
			return "", err
		}
		token = strings.TrimSpace(token)
		if token == "" {
			return "", errors.New("wrangler auth token is empty")
		}
		return token, nil
	default:
		return "", errors.New("authMode must be wrangler or token")
	}
}

func d1ExecutionMode(options map[string]any) string {
	mode := strings.ToLower(strings.TrimSpace(optionString(options, "executionMode")))
	switch mode {
	case d1ExecutionDev, d1ExecutionRemote:
		if mode == d1ExecutionDev && !d1DatasourceSupportsDev(options) {
			return d1ExecutionRemote
		}
		return mode
	}
	legacy := strings.ToLower(strings.TrimSpace(optionString(options, "mode")))
	switch legacy {
	case d1ModeLocal:
		return d1ExecutionDev
	case d1ModeCloud:
		return d1ExecutionRemote
	}
	return d1ExecutionRemote
}

func d1DatasourceSupportsDev(options map[string]any) bool {
	if strings.ToLower(strings.TrimSpace(optionString(options, "mode"))) == d1ModeLocal {
		return true
	}
	if strings.TrimSpace(optionString(options, "wranglerConfigPath")) != "" {
		return true
	}
	if !d1OptionBool(options, "supportDev") {
		return false
	}
	if strings.TrimSpace(optionString(options, "devProjectPath")) == "" {
		return false
	}
	return true
}

func d1OptionBool(options map[string]any, key string) bool {
	if options == nil {
		return false
	}
	raw, ok := options[key]
	if !ok || raw == nil {
		return false
	}
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func d1LocalCommand(ds datasource.DataSource, statement string) ([]string, func(), error) {
	databaseID := strings.TrimSpace(optionString(ds.Options, "databaseId"))
	if databaseID == "" {
		return nil, nil, errors.New("databaseId is required for d1")
	}
	databaseName := strings.TrimSpace(optionString(ds.Options, "databaseName"))
	if databaseName == "" {
		databaseName = strings.TrimSpace(ds.Database)
	}
	binding := strings.TrimSpace(optionString(ds.Options, "binding"))
	if binding == "" {
		binding = d1BindingFromDatabaseName(databaseName)
	}
	if databaseName == "" {
		databaseName = binding
	}
	if databaseName == "" {
		return nil, nil, errors.New("databaseName is required for dev mode")
	}

	command, err := d1WranglerBaseCommand(ds.Options)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {}
	configPath := strings.TrimSpace(optionString(ds.Options, "wranglerConfigPath"))
	if configPath == "" {
		path, remove, err := d1WriteTempWranglerConfig(ds, binding, databaseID)
		if err != nil {
			return nil, nil, err
		}
		configPath = path
		cleanup = remove
	}

	command = append(command,
		"d1", "execute", databaseName,
		"--local",
		"--json",
		"--command", statement,
	)
	if configPath != "" {
		command = append(command, "--config", configPath)
	}
	if persistPath := strings.TrimSpace(optionString(ds.Options, "persistPath")); persistPath != "" {
		command = append(command, "--persist-to", persistPath)
	}
	return command, cleanup, nil
}

func d1WriteTempWranglerConfig(ds datasource.DataSource, binding string, databaseID string) (string, func(), error) {
	databaseName := strings.TrimSpace(optionString(ds.Options, "databaseName"))
	if databaseName == "" {
		databaseName = strings.TrimSpace(ds.Database)
	}
	if databaseName == "" {
		databaseName = strings.ToLower(binding)
	}

	payload := map[string]any{
		"name":               "futrix-d1-local",
		"compatibility_date": "2024-01-01",
		"d1_databases": []map[string]any{
			{
				"binding":       binding,
				"database_name": databaseName,
				"database_id":   databaseID,
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	file, err := os.CreateTemp("", "futrix-d1-*.json")
	if err != nil {
		return "", nil, err
	}
	if _, err := file.Write(raw); err != nil {
		file.Close()
		_ = os.Remove(file.Name())
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, err
	}
	return file.Name(), func() {
		_ = os.Remove(file.Name())
	}, nil
}

func d1BindingFromDatabaseName(databaseName string) string {
	trimmed := strings.TrimSpace(strings.ToLower(databaseName))
	if trimmed == "" {
		return "db"
	}
	re := regexp.MustCompile(`[^a-z0-9_]+`)
	binding := re.ReplaceAllString(trimmed, "_")
	binding = strings.Trim(binding, "_")
	if binding == "" {
		return "db"
	}
	if binding[0] >= '0' && binding[0] <= '9' {
		return "db_" + binding
	}
	return binding
}

func d1WranglerBaseCommand(options map[string]any) ([]string, error) {
	defaultCommand := []string{"npx", "wrangler"}
	if options != nil {
		if raw, ok := options["wranglerCommand"]; ok && raw != nil {
			switch typed := raw.(type) {
			case string:
				fields := strings.Fields(strings.TrimSpace(typed))
				if len(fields) > 0 {
					if err := d1ValidateWranglerCommand(fields); err != nil {
						return nil, err
					}
					return fields, nil
				}
			case []string:
				out := make([]string, 0, len(typed))
				for _, item := range typed {
					item = strings.TrimSpace(item)
					if item == "" {
						continue
					}
					out = append(out, item)
				}
				if len(out) > 0 {
					if err := d1ValidateWranglerCommand(out); err != nil {
						return nil, err
					}
					return out, nil
				}
			case []any:
				out := make([]string, 0, len(typed))
				for _, item := range typed {
					text := strings.TrimSpace(fmt.Sprint(item))
					if text == "" || text == "<nil>" {
						continue
					}
					out = append(out, text)
				}
				if len(out) > 0 {
					if err := d1ValidateWranglerCommand(out); err != nil {
						return nil, err
					}
					return out, nil
				}
			}
		}
	}
	return defaultCommand, nil
}

func d1ValidateWranglerCommand(command []string) error {
	if len(command) == 0 {
		return errors.New("wrangler command is required")
	}
	if len(command) == 1 {
		base := strings.ToLower(filepath.Base(strings.TrimSpace(command[0])))
		if base == "wrangler" || base == "wrangler.cmd" || base == "wrangler.exe" {
			return nil
		}
		return errors.New("wrangler command must be `wrangler` or `npx wrangler`")
	}
	if len(command) == 2 {
		runner := strings.ToLower(strings.TrimSpace(command[0]))
		target := strings.ToLower(strings.TrimSpace(command[1]))
		if runner == "npx" && target == "wrangler" {
			return nil
		}
	}
	return errors.New("wrangler command must be `wrangler` or `npx wrangler`")
}

func d1RunCommand(ctx context.Context, command []string) ([]byte, error) {
	if len(command) == 0 {
		return nil, errors.New("command is required")
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	commandutil.ApplyStableWorkingDir(cmd)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		snippet := strings.TrimSpace(stderr.String())
		if snippet == "" {
			snippet = strings.TrimSpace(stdout.String())
		}
		if len(snippet) > 800 {
			snippet = snippet[:800] + "..."
		}
		if snippet != "" {
			return nil, fmt.Errorf("%w: %s", err, snippet)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

func d1ResolveWranglerToken(ctx context.Context, command []string, run func(context.Context, []string) ([]byte, error)) (string, error) {
	cmd := append([]string{}, command...)
	cmd = append(cmd, "auth", "token", "--json")
	raw, err := run(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("wrangler auth token failed: %w", err)
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("parse wrangler auth token: %w", err)
	}
	token := strings.TrimSpace(payload.Token)
	if token == "" {
		return "", errors.New("wrangler auth token is empty")
	}
	return token, nil
}

func d1DecodeStatementResults(raw []byte) ([]d1StatementResult, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var results []d1StatementResult
		if err := json.Unmarshal([]byte(trimmed), &results); err != nil {
			return nil, err
		}
		return results, nil
	}

	var envelope d1APIEnvelope
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil {
		if len(envelope.Result) > 0 || len(envelope.Errors) > 0 {
			if !envelope.Success && len(envelope.Errors) > 0 {
				return nil, errors.New(strings.TrimSpace(envelope.Errors[0].Message))
			}
			return envelope.Result, nil
		}
	}

	var single d1StatementResult
	if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
		return nil, err
	}
	return []d1StatementResult{single}, nil
}

func d1FirstStatementResult(results []d1StatementResult) d1StatementResult {
	if len(results) == 0 {
		return d1StatementResult{}
	}
	return results[0]
}

func d1ColumnsFromRows(rows []map[string]any) []string {
	if len(rows) == 0 {
		return nil
	}
	columns := make([]string, 0, len(rows[0]))
	for key := range rows[0] {
		columns = append(columns, key)
	}
	sort.Strings(columns)
	return columns
}

func d1ElapsedMs(meta map[string]any) int64 {
	if len(meta) == 0 {
		return 0
	}
	if raw, ok := meta["duration"]; ok {
		if val, ok := d1Float64(raw); ok {
			return int64(val)
		}
	}
	if raw, ok := meta["duration_ms"]; ok {
		if val, ok := d1Float64(raw); ok {
			return int64(val)
		}
	}
	if timingsRaw, ok := meta["timings"]; ok {
		if timings, ok := timingsRaw.(map[string]any); ok {
			if raw, ok := timings["sql_duration_ms"]; ok {
				if val, ok := d1Float64(raw); ok {
					return int64(val)
				}
			}
		}
	}
	return 0
}

func d1RowsWritten(meta map[string]any) int64 {
	if len(meta) == 0 {
		return 0
	}
	if raw, ok := meta["rows_written"]; ok {
		if value, ok := toInt64(raw); ok {
			return value
		}
	}
	return 0
}

func d1Float64(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case json.Number:
		if f, err := typed.Float64(); err == nil {
			return f, true
		}
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func d1IsQueryStatement(lowered string) bool {
	switch d1TopLevelVerb(lowered) {
	case "select", "show", "describe", "explain":
		return true
	default:
		return false
	}
}

func d1ClampPageSize(pageSize int) int {
	if pageSize <= 0 {
		pageSize = 200
	}
	max := int(window.LimitPolicy{Max: window.DefaultLimit}.Decide(nil).Effective)
	if pageSize > max {
		return max
	}
	return pageSize
}

func d1TopLevelVerb(lowered string) string {
	return SQLStatementVerb(lowered, "d1")
}

func d1TrimLeadingSQLComments(statement string) string {
	trimmed := strings.TrimLeft(statement, " \t\n\r")
	for trimmed != "" {
		if strings.HasPrefix(trimmed, "--") {
			newline := strings.IndexByte(trimmed, '\n')
			if newline < 0 {
				return ""
			}
			trimmed = strings.TrimLeft(trimmed[newline+1:], " \t\n\r")
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			end := strings.Index(trimmed, "*/")
			if end < 0 {
				return ""
			}
			trimmed = strings.TrimLeft(trimmed[end+2:], " \t\n\r")
			continue
		}
		break
	}
	return trimmed
}

func d1StripSQLComments(statement string) string {
	var builder strings.Builder
	builder.Grow(len(statement))

	inSingle := false
	inDouble := false
	inBacktick := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < len(statement); i++ {
		ch := statement[i]
		if inLineComment {
			if ch == '\n' || ch == '\r' {
				inLineComment = false
				builder.WriteByte(ch)
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && i+1 < len(statement) && statement[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if inSingle {
			builder.WriteByte(ch)
			if ch == '\\' && i+1 < len(statement) {
				i++
				builder.WriteByte(statement[i])
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			builder.WriteByte(ch)
			if ch == '\\' && i+1 < len(statement) {
				i++
				builder.WriteByte(statement[i])
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		if inBacktick {
			builder.WriteByte(ch)
			if ch == '`' {
				inBacktick = false
			}
			continue
		}

		if ch == '-' && i+1 < len(statement) && statement[i+1] == '-' {
			inLineComment = true
			i++
			continue
		}
		if ch == '/' && i+1 < len(statement) && statement[i+1] == '*' {
			inBlockComment = true
			i++
			continue
		}

		switch ch {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '`':
			inBacktick = true
		}
		builder.WriteByte(ch)
	}
	return builder.String()
}

func d1StatementHash(statement string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(statement)))
	return fmt.Sprintf("%x", sum[:])
}

func d1EncodePagingToken(token d1PagingToken) (string, error) {
	raw, err := json.Marshal(token)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func d1DecodePagingToken(token string) (d1PagingToken, error) {
	var parsed d1PagingToken
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return parsed, err
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return parsed, err
	}
	if parsed.Version <= 0 {
		parsed.Version = 1
	}
	return parsed, nil
}

func d1AppendLimitOffset(statement string, limit int, offset int) string {
	trimmed := strings.TrimRight(statement, " \t\n\r")
	suffix := ""
	if strings.HasSuffix(trimmed, ";") {
		trimmed = strings.TrimRight(trimmed[:len(trimmed)-1], " \t")
		suffix = ";"
	}
	separator := " "
	if d1HasTrailingLineComment(trimmed) {
		separator = "\n"
	}
	return fmt.Sprintf("%s%sLIMIT %d OFFSET %d%s", trimmed, separator, limit, offset, suffix)
}

func d1HasTrailingLineComment(statement string) bool {
	inSingle := false
	inDouble := false
	inBacktick := false
	inLineComment := false

	for i := 0; i < len(statement); i++ {
		ch := statement[i]
		if inLineComment {
			if ch == '\n' || ch == '\r' {
				inLineComment = false
			}
			continue
		}
		if inSingle {
			if ch == '\\' && i+1 < len(statement) {
				i++
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '\\' && i+1 < len(statement) {
				i++
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		if inBacktick {
			if ch == '`' {
				inBacktick = false
			}
			continue
		}

		switch ch {
		case '\'':
			inSingle = true
			continue
		case '"':
			inDouble = true
			continue
		case '`':
			inBacktick = true
			continue
		}

		if ch == '/' && i+1 < len(statement) && statement[i+1] == '*' {
			i += 2
			for i < len(statement)-1 && !(statement[i] == '*' && statement[i+1] == '/') {
				i++
			}
			if i < len(statement)-1 {
				i++
			}
			continue
		}

		if ch == '-' && i+1 < len(statement) && statement[i+1] == '-' {
			inLineComment = true
			i++
		}
	}
	return inLineComment
}

func d1BuildListEntitiesStatement(pattern string, cursor string, limit int) string {
	builder := strings.Builder{}
	builder.WriteString("SELECT name, type FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '\\_cf\\_%' ESCAPE '\\'")
	if like := d1PatternLike(pattern); like != "" {
		builder.WriteString(" AND name LIKE '")
		builder.WriteString(d1QuoteSQLString(like))
		builder.WriteString("'")
	}
	if cur := strings.TrimSpace(cursor); cur != "" {
		builder.WriteString(" AND name > '")
		builder.WriteString(d1QuoteSQLString(cur))
		builder.WriteString("'")
	}
	builder.WriteString(" ORDER BY name")
	if limit > 0 {
		builder.WriteString(fmt.Sprintf(" LIMIT %d", limit))
	}
	return builder.String()
}

func d1PatternLike(pattern string) string {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return ""
	}
	like := strings.ReplaceAll(trimmed, "*", "%")
	if !strings.ContainsAny(like, "%_") {
		like = "%" + like + "%"
	}
	return like
}

func d1QuoteSQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func d1IsHiddenSystemEntity(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "sqlite_") || strings.HasPrefix(lower, "_cf_")
}

func d1IsSQLiteAuthError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToUpper(err.Error()), "SQLITE_AUTH")
}

func d1DetailUsesIndex(detail string) bool {
	lower := strings.ToLower(strings.TrimSpace(detail))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "using covering index") ||
		strings.Contains(lower, "using index") ||
		strings.Contains(lower, "using integer primary key") ||
		strings.Contains(lower, "using primary key")
}

func d1HasQueryPlanPrefix(statement string) bool {
	rest, ok := cutLeadingKeyword(statement, "query")
	if !ok {
		return false
	}
	_, ok = cutLeadingKeyword(rest, "plan")
	return ok
}

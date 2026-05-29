package console

import (
	"fmt"
	"strings"
)

// normalizeForParse applies dialect-specific normalization to make SQL
// compatible with the PostgreSQL parser. For postgres dialect, returns
// the input unchanged.
func normalizeForParse(sql, dialect string) string {
	if dialect != "mysql" {
		return sql
	}
	return mysqlToPostgres(sql)
}

// mysqlToPostgres converts MySQL-specific syntax to PostgreSQL-compatible
// syntax for parsing purposes only (not for execution).
func mysqlToPostgres(sql string) string {
	sql = normalizeMySQLHashComments(sql)
	sql = replaceBackticks(sql)
	sql = convertMySQLLimitSyntax(sql)
	return sql
}

func normalizeMySQLHashComments(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))
	inSingle := false
	inDouble := false
	inBacktick := false
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if inSingle {
			b.WriteByte(ch)
			if ch == '\\' && i+1 < len(sql) {
				i++
				b.WriteByte(sql[i])
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			b.WriteByte(ch)
			if ch == '\\' && i+1 < len(sql) {
				i++
				b.WriteByte(sql[i])
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		if inBacktick {
			b.WriteByte(ch)
			if ch == '`' {
				inBacktick = false
			}
			continue
		}

		switch ch {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '`':
			inBacktick = true
		case '#':
			b.WriteString("--")
			for i+1 < len(sql) && sql[i+1] != '\n' {
				i++
				b.WriteByte(sql[i])
			}
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

// replaceBackticks converts MySQL backtick-quoted identifiers to
// PostgreSQL double-quoted identifiers. Backticks inside single-quoted
// strings are left untouched.
func replaceBackticks(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))
	inSingle := false
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if inSingle {
			b.WriteByte(ch)
			if ch == '\\' && i+1 < len(sql) {
				i++
				b.WriteByte(sql[i])
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if ch == '\'' {
			inSingle = true
			b.WriteByte(ch)
			continue
		}
		if ch == '`' {
			b.WriteByte('"')
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

// convertMySQLLimitSyntax converts MySQL's LIMIT offset, count syntax
// to PostgreSQL's LIMIT count OFFSET offset. Uses the existing
// findTopLevelLimit which already detects the offsetFirst pattern.
func convertMySQLLimitSyntax(sql string) string {
	info := findTopLevelLimit(sql)
	if !info.found || !info.parsed || !info.offsetFirst {
		return sql
	}
	replacement := fmt.Sprintf("LIMIT %d OFFSET %d", info.count, info.offset)
	return sql[:info.start] + replacement + sql[info.end:]
}

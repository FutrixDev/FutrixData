package console

import (
	"fmt"
	"regexp"
	"strings"
)

var sqlViewDefinitionPattern = regexp.MustCompile(`(?is)^\s*create\s+(?:temp(?:orary)?\s+)?view\b(?:\s+if\s+not\s+exists\b)?\s+.+?\bas\b\s+(select\b.+)$`)

type SQLViewRiskFacts struct {
	ViewEntity    string
	BaseEntities  []string
	EntityNameMap map[string]string
	Inner         SQLRiskFacts
}

func SQLViewRiskFactsForDefinition(viewEntity, definition, dialect string) (SQLViewRiskFacts, error) {
	viewEntity = normalizeSQLRiskEntity(viewEntity)
	definition = strings.TrimSpace(definition)
	if viewEntity == "" {
		return SQLViewRiskFacts{}, fmt.Errorf("view entity required")
	}
	if definition == "" {
		return SQLViewRiskFacts{}, fmt.Errorf("view definition required")
	}

	match := sqlViewDefinitionPattern.FindStringSubmatch(definition)
	if len(match) < 2 {
		return SQLViewRiskFacts{}, fmt.Errorf("view definition does not contain a selectable query")
	}
	innerSQL := strings.TrimSpace(match[1])

	analysis, err := analyzeSQLForDialect(innerSQL, dialect)
	if err != nil {
		return SQLViewRiskFacts{}, fmt.Errorf("analyze view definition: %w", err)
	}
	inner := sqlRiskFactsFromAnalysis(analysis)
	inner = mergeFallbackSQLRiskFacts(inner, fallbackSQLRiskFacts(innerSQL, dialect))
	if inner.Verb != "select" {
		return SQLViewRiskFacts{}, fmt.Errorf("unsupported view statement verb %q", inner.Verb)
	}

	baseEntities := uniqueSQLTableNames(analysis.Tables)
	baseEntities = filterCTEEntityNames(baseEntities, analysis.CTENames)
	baseEntities = prependPrimaryEntity(inner.TargetEntity, baseEntities)

	return SQLViewRiskFacts{
		ViewEntity:    viewEntity,
		BaseEntities:  baseEntities,
		EntityNameMap: sqlViewEntityNameMap(analysis.Tables, analysis.CTENames),
		Inner:         inner,
	}, nil
}

func sqlViewEntityNameMap(refs []TableRef, cteNames []string) map[string]string {
	if len(refs) == 0 {
		return nil
	}
	out := make(map[string]string, len(refs)*3)
	for _, ref := range refs {
		base := normalizeSQLRiskEntity(tableRefName(ref))
		if base == "" || isCTEName(base, cteNames) {
			continue
		}
		out[base] = base
		if trimmedTable := normalizeSQLRiskEntity(ref.Table); trimmedTable != "" {
			out[trimmedTable] = base
		}
		if alias := normalizeSQLRiskEntity(ref.Alias); alias != "" {
			out[alias] = base
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

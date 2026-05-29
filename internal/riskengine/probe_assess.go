package riskengine

import (
	"fmt"
	"math"
	"strings"

	"futrixdata/platform/internal/console"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	probeRuleViewVerification = Rule{ID: "probe-view-verification", Code: "PRB-001", Description: "Warn when a view definition or expanded query cannot be verified", Builtin: true}
	probeRuleExecutionPath    = Rule{ID: "probe-execution-path", Code: "PRB-002", Description: "Warn when the execution path cannot be verified", Builtin: true}
	probeRuleNoIndex          = Rule{ID: "probe-no-index", Code: "PRB-003", Description: "Warn when the execution plan does not show index usage", Builtin: true}
	probeRuleWideScan         = Rule{ID: "probe-wide-scan", Code: "PRB-004", Description: "Warn when the execution plan examines too many rows", Builtin: true}
	probeRulePlanRisk         = Rule{ID: "probe-plan-risk", Code: "PRB-005", Description: "Warn when the execution plan shows scan-heavy or complex access paths", Builtin: true}
	probeRuleMetadataMissing  = Rule{ID: "probe-metadata-missing", Code: "PRB-006", Description: "Warn when metadata required for risk checks is unavailable", Builtin: true}
	probeRuleAccessPath       = Rule{ID: "probe-access-path", Code: "PRB-007", Description: "Warn when the access path cannot be verified from available metadata", Builtin: true}
)

// AssessWithProbe enhances a base risk assessment with EXPLAIN/probe results.
// This mirrors the logic from aichat assessExecuteStatementRisk.
func AssessWithProbe(base RiskAssessment, ps ParsedStatement, probe ProbeResult) RiskAssessment {
	return AssessWithProbePolicy(base, ps, probe, DefaultProbePolicy())
}

func AssessWithProbePolicy(base RiskAssessment, ps ParsedStatement, probe ProbeResult, policy ProbePolicy) RiskAssessment {
	// Only enhance if base assessment is allow (low risk)
	if base.Action != ActionAllow && base.Action != ActionWarn {
		return base
	}
	policy = policy.normalized()

	switch ps.DsType {
	case "mysql", "postgresql", "d1", "mongodb":
		return assessSQLMongoProbe(base, ps, probe, policy)
	case "elasticsearch":
		return assessESProbe(base, ps)
	case "dynamodb":
		return assessDynamoProbe(base, ps, probe)
	default:
		return base
	}
}

func assessSQLMongoProbe(base RiskAssessment, ps ParsedStatement, probe ProbeResult, policy ProbePolicy) RiskAssessment {
	if probe.ExplainSkipped {
		if d1ViewVerificationUnverified(ps, probe) {
			return escalateToWarnWithRule(base, probeRuleViewVerification, "view definition not verified")
		}
		return base
	}
	if probe.ExplainErr != nil {
		return escalateToWarnWithRule(base, probeRuleExecutionPath, "execution path not verified")
	}
	if probe.ExplainResult == nil {
		return escalateToWarnWithRule(base, probeRuleExecutionPath, "execution path not verified")
	}
	explain := *probe.ExplainResult

	result := base
	if d1ViewVerificationUnverified(ps, probe) {
		result = escalateToWarnWithRule(result, probeRuleViewVerification, "view definition not verified")
	}

	// Basic checks
	viewReasons := d1ViewExplainReasons(ps, probe, explain)
	if len(viewReasons) > 0 {
		for _, reason := range viewReasons {
			result = escalateToWarnWithRule(result, probeRulePlanRisk, reason)
		}
	} else if !explain.UsesIndex && !(policy.AllowSafeSeqScan && seqScanIsSafe(explain, policy)) {
		result = escalateToWarnWithRule(result, probeRuleNoIndex, "no index detected")
	}
	examined := examinedScope(explain)
	if examined <= 0 && explain.UsesIndex && !probeUsesStructuralPlanSignals(ps, explain) {
		result = escalateToWarnWithRule(result, probeRuleExecutionPath, "execution path not verified")
	}
	// Use the higher SeqScanRowsThreshold when the plan is a safe small-table scan,
	// so the examined-rows gate doesn't contradict the seq-scan exemption.
	examinedThreshold := policy.MaxExaminedRows
	if policy.AllowSafeSeqScan && seqScanIsSafe(explain, policy) {
		examinedThreshold = policy.SeqScanRowsThreshold
	}
	if examined > examinedThreshold {
		result = escalateToWarnWithRule(result, probeRuleWideScan, fmt.Sprintf("examined over %d rows", examinedThreshold))
	}

	// Deep EXPLAIN analysis from Detail
	risks := AnalyzeExplainDetailWithPolicy(explain, policy)
	for _, reason := range risks {
		result = escalateToWarnWithRule(result, probeRulePlanRisk, reason)
	}

	return result
}

func d1ViewVerificationUnverified(ps ParsedStatement, probe ProbeResult) bool {
	if !d1ViewVerificationRequired(ps, probe) {
		return false
	}
	return probe.ViewParseErr != nil || probe.DescribeErr != nil
}

func d1ViewVerificationRequired(ps ParsedStatement, probe ProbeResult) bool {
	if ps.DsType != "d1" || strings.TrimSpace(ps.TargetEntity) == "" {
		return false
	}
	if probe.ViewResult != nil || probe.ViewParseErr != nil {
		return true
	}
	if probe.DescribeResult != nil && strings.EqualFold(strings.TrimSpace(probe.DescribeResult.EntityKind), "view") {
		return true
	}
	if probe.DescribeErr != nil {
		if probe.ExplainResult == nil {
			return false
		}
		return d1ExplainContainsViewNode(ps.TargetEntity, probe.ExplainResult) || !d1ExplainReferencesDirectTarget(ps, probe.ExplainResult)
	}
	return false
}

func d1ExplainContainsViewNode(target string, explain *console.ExplainResult) bool {
	if explain == nil {
		return false
	}
	target = normalizeD1ExplainEntityToken(target)
	if target == "" {
		return false
	}
	for _, stage := range explain.Stages {
		trimmed := strings.TrimSpace(stage)
		upper := strings.ToUpper(trimmed)
		switch {
		case strings.HasPrefix(upper, "CO-ROUTINE "):
			entity := strings.TrimSpace(trimmed[len("CO-ROUTINE "):])
			parts := strings.Fields(entity)
			if len(parts) == 0 {
				continue
			}
			if normalizeD1ExplainEntityToken(parts[0]) == target {
				return true
			}
		case strings.HasPrefix(upper, "MATERIALIZE "):
			entity := strings.TrimSpace(trimmed[len("MATERIALIZE "):])
			parts := strings.Fields(entity)
			if len(parts) == 0 {
				continue
			}
			if normalizeD1ExplainEntityToken(parts[0]) == target {
				return true
			}
		}
	}
	return false
}

func d1ExplainReferencesDirectTarget(ps ParsedStatement, explain *console.ExplainResult) bool {
	if explain == nil {
		return false
	}
	expected := d1ExpectedExplainTargetTokens(ps)
	if len(expected) == 0 {
		return false
	}
	for _, stage := range explain.Stages {
		token, ok := d1ExplainPrimaryEntityToken(stage)
		if !ok {
			continue
		}
		if _, exists := expected[token]; exists {
			return true
		}
	}
	return false
}

func d1ExpectedExplainTargetTokens(ps ParsedStatement) map[string]struct{} {
	tokens := make(map[string]struct{})
	addD1ExplainTargetToken(tokens, ps.TargetEntity)

	analysis, err := console.AnalyzeSQLForDialect(ps.Raw, ps.DsType)
	if err != nil || analysis == nil {
		return tokens
	}
	target := normalizeSQLTargetEntityToken(ps.TargetEntity)
	if target == "" {
		return tokens
	}
	targetLeaf := normalizeSQLTargetEntityToken(d1TargetLeafName(ps.TargetEntity))
	for _, ref := range analysis.Tables {
		fullName := normalizeSQLTargetEntityToken(d1AnalysisTableRefName(ref))
		baseName := normalizeSQLTargetEntityToken(ref.Table)
		if fullName != target && baseName != target && baseName != targetLeaf {
			continue
		}
		addD1ExplainTargetToken(tokens, ref.Alias)
		addD1ExplainTargetToken(tokens, ref.Table)
		addD1ExplainTargetToken(tokens, d1AnalysisTableRefName(ref))
	}
	return tokens
}

func addD1ExplainTargetToken(tokens map[string]struct{}, value string) {
	token := normalizeSQLTargetEntityToken(value)
	if token == "" {
		return
	}
	tokens[token] = struct{}{}
	if leaf := normalizeSQLTargetEntityToken(d1TargetLeafName(token)); leaf != "" {
		tokens[leaf] = struct{}{}
	}
}

func normalizeSQLTargetEntityToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToLower(value)
}

func d1TargetLeafName(value string) string {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func d1AnalysisTableRefName(ref console.TableRef) string {
	if strings.TrimSpace(ref.Schema) == "" {
		return strings.TrimSpace(ref.Table)
	}
	return strings.TrimSpace(ref.Schema) + "." + strings.TrimSpace(ref.Table)
}

func d1ExplainPrimaryEntityToken(stage string) (string, bool) {
	trimmed := strings.TrimSpace(stage)
	upper := strings.ToUpper(trimmed)
	var entity string
	switch {
	case strings.HasPrefix(upper, "SEARCH "):
		entity = strings.TrimSpace(trimmed[len("SEARCH "):])
	case strings.HasPrefix(upper, "SCAN "):
		entity = strings.TrimSpace(trimmed[len("SCAN "):])
	case strings.HasPrefix(upper, "CO-ROUTINE "):
		entity = strings.TrimSpace(trimmed[len("CO-ROUTINE "):])
	case strings.HasPrefix(upper, "MATERIALIZE "):
		entity = strings.TrimSpace(trimmed[len("MATERIALIZE "):])
	default:
		return "", false
	}
	parts := strings.Fields(entity)
	if len(parts) == 0 {
		return "", false
	}
	return normalizeD1ExplainEntityToken(parts[0]), true
}

func d1ViewExplainReasons(ps ParsedStatement, probe ProbeResult, explain console.ExplainResult) []string {
	if ps.DsType != "d1" || probe.ViewResult == nil {
		return nil
	}
	var reasons []string
	for _, stage := range explain.Stages {
		trimmed := strings.TrimSpace(stage)
		upper := strings.ToUpper(trimmed)
		switch {
		case strings.HasPrefix(upper, "SCAN "):
			entity := strings.TrimSpace(trimmed[len("SCAN "):])
			parts := strings.Fields(entity)
			if len(parts) == 0 {
				continue
			}
			entity = normalizeD1ExplainEntityToken(parts[0])
			if baseEntity := resolveD1ViewPlanEntity(probe.ViewResult, entity); baseEntity != "" {
				reasons = append(reasons, fmt.Sprintf("view expands to scan on %s", baseEntity))
			}
		case strings.Contains(upper, "USE TEMP B-TREE FOR GROUP BY"):
			reasons = append(reasons, "view query builds temporary grouping structure")
		case strings.Contains(upper, "USE TEMP B-TREE FOR ORDER BY"):
			reasons = append(reasons, "view query builds temporary sort structure")
		case strings.HasPrefix(upper, "MATERIALIZE "):
			reasons = append(reasons, "view query materializes intermediate result")
		}
	}
	return uniqueReasons(reasons)
}

func resolveD1ViewPlanEntity(view *ViewProbeResult, entity string) string {
	entity = normalizeD1ExplainEntityToken(entity)
	if entity == "" || view == nil {
		return ""
	}
	if base, ok := view.EntityNameMap[entity]; ok && base != "" {
		return base
	}
	if containsFold(view.BaseEntities, entity) {
		return strings.TrimSpace(entity)
	}
	return ""
}

func normalizeD1ExplainEntityToken(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"`'(),")
	value = strings.TrimSpace(value)
	return strings.ToLower(value)
}

func probeUsesStructuralPlanSignals(ps ParsedStatement, explain console.ExplainResult) bool {
	if ps.DsType != "d1" {
		return false
	}
	if d1PlanMixesIndexAndScan(explain) {
		return false
	}
	return len(explain.Stages) > 0 || explain.Detail != nil
}

func d1PlanMixesIndexAndScan(explain console.ExplainResult) bool {
	hasSearch := false
	hasScan := false
	for _, stage := range explain.Stages {
		trimmed := strings.TrimSpace(stage)
		upper := strings.ToUpper(trimmed)
		switch {
		case strings.HasPrefix(upper, "SEARCH "):
			hasSearch = true
		case strings.HasPrefix(upper, "SCAN "):
			hasScan = true
		}
		if hasSearch && hasScan {
			return true
		}
	}
	return false
}

func containsFold(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func uniqueReasons(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

// AnalyzeExplainDetail extracts risk signals from the raw EXPLAIN output.
// Works with both MySQL ([]map[string]any) and PostgreSQL (JSON plan tree) formats.
func AnalyzeExplainDetail(explain console.ExplainResult) []string {
	return AnalyzeExplainDetailWithPolicy(explain, DefaultProbePolicy())
}

func AnalyzeExplainDetailWithPolicy(explain console.ExplainResult, policy ProbePolicy) []string {
	policy = policy.normalized()
	if explain.Detail == nil {
		return nil
	}

	// Try MySQL format: rows with select_type/table/type fields
	if rows, ok := toMapSlice(explain.Detail); ok && len(rows) > 0 {
		if isMySQLExplainRows(rows) {
			return analyzeMySQLExplain(rows, policy)
		}
	}

	if isMongoExplainDetail(explain) {
		return analyzeMongoExplain(explain, policy)
	}

	// PostgreSQL format: JSON plan tree with Plan/Node Type
	return analyzePostgresExplain(explain.Detail, policy)
}

// isMySQLExplainRows checks if the data looks like MySQL EXPLAIN output.
func isMySQLExplainRows(rows []map[string]any) bool {
	if len(rows) == 0 {
		return false
	}
	first := rows[0]
	// MySQL EXPLAIN has select_type or table; PostgreSQL has Plan
	_, hasSelectType := first["select_type"]
	_, hasTable := first["table"]
	_, hasPlan := first["Plan"]
	return (hasSelectType || hasTable) && !hasPlan
}

func analyzeMySQLExplain(rows []map[string]any, policy ProbePolicy) []string {
	var risks []string

	// Count tables involved (each row = one table access)
	tableCount := len(rows)
	if tableCount > policy.MaxJoinCount+1 {
		risks = append(risks, fmt.Sprintf("%d tables joined", tableCount))
	}

	fullScanCount := 0
	hasSubquery := false
	hasDependentSubquery := false
	hasTempTable := false
	hasFilesort := false
	var totalEstimatedRows int64 = 1

	for _, row := range rows {
		selectType := strings.ToLower(strings.TrimSpace(asString(row["select_type"])))
		typ := strings.ToUpper(strings.TrimSpace(asString(row["type"])))
		extra := strings.ToLower(strings.TrimSpace(asString(row["Extra"])))
		rowsStr := strings.TrimSpace(asString(row["rows"]))

		// Subquery detection
		switch selectType {
		case "subquery", "derived", "materialized":
			hasSubquery = true
		case "dependent subquery", "dependent union", "uncacheable subquery":
			hasDependentSubquery = true
		}

		// Full table scan count
		if typ == "ALL" {
			fullScanCount++
		}

		// Temporary table / filesort in Extra
		if strings.Contains(extra, "using temporary") {
			hasTempTable = true
		}
		if strings.Contains(extra, "using filesort") {
			hasFilesort = true
		}

		// Multiply estimated rows for JOIN cost estimation
		if rowsStr != "" {
			if rowsValue, err := parseInt64(rowsStr); err == nil && rowsValue > 0 {
				totalEstimatedRows = safeMultiplyInt64(totalEstimatedRows, rowsValue)
			}
		}
	}

	if fullScanCount > policy.MaxFullScans {
		risks = append(risks, fmt.Sprintf("%d full table scans (possible cartesian product)", fullScanCount))
	}
	if hasDependentSubquery {
		risks = append(risks, "dependent subquery (executes per row)")
	} else if hasSubquery {
		risks = append(risks, "contains subquery")
	}
	if hasTempTable {
		risks = append(risks, "uses temporary table")
	}
	if hasFilesort {
		risks = append(risks, "uses filesort")
	}
	// For JOINs, total rows = product of rows per table
	if tableCount > 1 && totalEstimatedRows > policy.MaxEstimatedJoinRows {
		risks = append(risks, fmt.Sprintf("estimated %d rows across joins", totalEstimatedRows))
	}

	return risks
}

func isMongoExplainDetail(explain console.ExplainResult) bool {
	if mongoExplainHasTopLevelKey(explain.Detail, "queryPlanner") || mongoExplainHasTopLevelKey(explain.Detail, "executionStats") {
		return true
	}
	for _, stage := range console.MongoExplainPlanSummaryForResult(explain).Stages {
		switch stage {
		case "COLLSCAN", "IXSCAN", "FETCH", "IDHACK", "SHARD_MERGE", "EQ_LOOKUP", "HASH_LOOKUP":
			return true
		}
	}
	return false
}

func analyzeMongoExplain(explain console.ExplainResult, policy ProbePolicy) []string {
	var risks []string
	stageCounts := console.MongoExplainPlanSummaryForResult(explain).StageCounts

	if collScans := stageCounts["COLLSCAN"]; collScans > policy.MaxFullScans {
		risks = append(risks, fmt.Sprintf("%d collection scans in MongoDB plan", collScans))
	} else if collScans > 0 && explain.TotalDocsExamined > policy.MaxExaminedRows {
		risks = append(risks, fmt.Sprintf("collection scan over %d docs", policy.MaxExaminedRows))
	}

	if sortStages := stageCounts["SORT"]; sortStages > 0 && explain.TotalDocsExamined > policy.MaxExaminedRows {
		risks = append(risks, fmt.Sprintf("blocking sort over %d docs", policy.MaxExaminedRows))
	}

	if joins := stageCounts["EQ_LOOKUP"] + stageCounts["HASH_LOOKUP"]; joins > 0 && explain.TotalDocsExamined > policy.MaxExaminedRows {
		risks = append(risks, fmt.Sprintf("aggregation join examines over %d docs", policy.MaxExaminedRows))
	}

	return risks
}

func mongoExplainHasTopLevelKey(detail any, key string) bool {
	switch typed := detail.(type) {
	case map[string]any:
		_, ok := typed[key]
		return ok
	case bson.M:
		_, ok := typed[key]
		return ok
	case bson.D:
		for _, item := range typed {
			if item.Key == key {
				return true
			}
		}
	}
	return false
}

func analyzePostgresExplain(detail any, policy ProbePolicy) []string {
	var risks []string

	joinCount := 0
	seqScanCount := 0
	hasSortOnLargeSet := false
	hasSubplan := false
	var maxPlanRows int64

	walkPGPlan(detail, func(node map[string]any) {
		nodeType := strings.ToLower(strings.TrimSpace(asString(node["Node Type"])))

		// Count JOINs
		switch {
		case strings.Contains(nodeType, "nested loop"),
			strings.Contains(nodeType, "hash join"),
			strings.Contains(nodeType, "merge join"):
			joinCount++
		}

		// Count sequential scans
		if strings.Contains(nodeType, "seq scan") {
			seqScanCount++
		}

		// Detect subplans
		if _, ok := node["Subplan Name"]; ok {
			hasSubplan = true
		}

		// Check for Sort on large estimated rows
		if strings.Contains(nodeType, "sort") {
			if planRows, ok := toInt64Value(node["Plan Rows"]); ok && planRows > policy.MaxExaminedRows {
				hasSortOnLargeSet = true
			}
		}

		// Track max rows
		if planRows, ok := toInt64Value(node["Plan Rows"]); ok && planRows > maxPlanRows {
			maxPlanRows = planRows
		}
	})

	if joinCount > policy.MaxJoinCount {
		risks = append(risks, fmt.Sprintf("%d joins detected", joinCount))
	}
	if seqScanCount > policy.MaxFullScans {
		risks = append(risks, fmt.Sprintf("%d sequential scans", seqScanCount))
	}
	if hasSubplan {
		risks = append(risks, "contains subplan")
	}
	if hasSortOnLargeSet {
		risks = append(risks, fmt.Sprintf("sort on large result set (>%d rows)", policy.MaxExaminedRows))
	}

	return risks
}

// walkPGPlan walks a PostgreSQL EXPLAIN JSON tree, calling visit for each plan node.
func walkPGPlan(detail any, visit func(map[string]any)) {
	switch typed := detail.(type) {
	case []any:
		for _, item := range typed {
			walkPGPlan(item, visit)
		}
	case map[string]any:
		if plan, ok := typed["Plan"]; ok {
			if root, ok := plan.(map[string]any); ok {
				walkPGPlanNode(root, visit)
				return
			}
		}
		if _, ok := typed["Node Type"]; ok {
			walkPGPlanNode(typed, visit)
			return
		}
		for _, v := range typed {
			walkPGPlan(v, visit)
		}
	}
}

func walkPGPlanNode(node map[string]any, visit func(map[string]any)) {
	visit(node)
	if plans, ok := node["Plans"]; ok {
		if list, ok := plans.([]any); ok {
			for _, child := range list {
				if childNode, ok := child.(map[string]any); ok {
					walkPGPlanNode(childNode, visit)
				}
			}
		}
	}
}

func assessESProbe(base RiskAssessment, ps ParsedStatement) RiskAssessment {
	// For non-search requests (PUT, DELETE, etc.), no body analysis needed
	if ps.HTTPMethod != "POST" && ps.HTTPMethod != "GET" && ps.HTTPMethod != "HEAD" {
		return base
	}

	// GET/HEAD for specific docs are always safe
	if (ps.HTTPMethod == "GET" || ps.HTTPMethod == "HEAD") && strings.Contains(ps.URLPath, "/_doc/") {
		return base
	}

	// Run granular body analysis
	risks := append([]string{}, ElasticsearchPathQueryRisks(ps.URLPath)...)
	risks = append(risks, ElasticsearchBodyRisks(ps.Body)...)
	if len(risks) == 0 {
		return base
	}

	// Escalate with all detected risks
	result := base
	for _, reason := range risks {
		result = escalateToWarnWithRule(result, probeRulePlanRisk, reason)
	}
	return result
}

func assessDynamoProbe(base RiskAssessment, ps ParsedStatement, probe ProbeResult) RiskAssessment {
	if probe.DescribeErr != nil {
		return escalateToWarnWithRule(base, probeRuleMetadataMissing, "DynamoDB table metadata not available for access-path verification")
	}
	var describe any
	if probe.DescribeResult != nil {
		describe = probe.DescribeResult
	}
	assessment := AnalyzeDynamodbStatementAccess(ps.Raw, describe, probe.DescribeEntity)
	if assessment.Safe {
		return base
	}
	if strings.TrimSpace(assessment.Reason) != "" {
		return escalateToWarnWithRule(base, probeRuleAccessPath, assessment.Reason)
	}
	return escalateToWarnWithRule(base, probeRuleAccessPath, "access path not verified")
}

func escalateToWarn(base RiskAssessment, reason string) RiskAssessment {
	reasons := append([]string{}, base.Reasons...)
	trimmed := strings.TrimSpace(reason)
	if trimmed != "" {
		duplicate := false
		for _, existing := range reasons {
			if existing == trimmed {
				duplicate = true
				break
			}
		}
		if !duplicate {
			reasons = append(reasons, trimmed)
		}
	}
	return RiskAssessment{
		Level:           RiskMedium,
		Action:          ActionWarn,
		Reasons:         reasons,
		RuleID:          base.RuleID,
		RuleCode:        base.RuleCode,
		RuleDescription: base.RuleDescription,
		Builtin:         base.Builtin,
	}
}

func escalateToWarnWithRule(base RiskAssessment, rule Rule, reason string) RiskAssessment {
	result := escalateToWarn(base, reason)
	if base.Action == ActionAllow || strings.TrimSpace(base.RuleID) == "" {
		result.RuleID = rule.ID
		result.RuleCode = rule.Code
		result.RuleDescription = rule.Description
		result.Builtin = rule.Builtin
	}
	return result
}

func safeMultiplyInt64(a, b int64) int64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a > math.MaxInt64/b {
		return math.MaxInt64
	}
	return a * b
}

// seqScanIsSafe returns true when a sequential/full table scan is considered
// normal optimizer behavior: the scanned rows are within the community-standard
// threshold (default 10,000) AND the total cost is low (default < 1,000).
// Both conditions must hold for the scan to be considered safe.
func seqScanIsSafe(explain console.ExplainResult, policy ProbePolicy) bool {
	if explain.MaxSeqScanRows <= 0 {
		return false
	}
	if explain.MaxSeqScanRows > policy.SeqScanRowsThreshold {
		return false
	}
	// TotalCost == 0 means cost info is unavailable (e.g., MySQL); skip the cost check.
	if explain.TotalCost > 0 && explain.TotalCost >= policy.CostThreshold {
		return false
	}
	return true
}

func examinedScope(explain console.ExplainResult) int64 {
	examined := explain.TotalDocsExamined
	if explain.TotalKeysExamined > examined {
		examined = explain.TotalKeysExamined
	}
	return examined
}

// toMapSlice attempts to convert a value to []map[string]any (MySQL EXPLAIN format).
func toMapSlice(v any) ([]map[string]any, bool) {
	slice, ok := v.([]map[string]any)
	if ok {
		return slice, true
	}
	// json.Unmarshal produces []any, not []map[string]any
	anySlice, ok := v.([]any)
	if !ok {
		return nil, false
	}
	result := make([]map[string]any, 0, len(anySlice))
	for _, item := range anySlice {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		result = append(result, m)
	}
	return result, true
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func toInt64Value(v any) (int64, bool) {
	if v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	case string:
		if n, err := parseInt64(n); err == nil {
			return n, true
		}
	}
	return 0, false
}

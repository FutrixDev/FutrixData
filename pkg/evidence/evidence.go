package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/FutrixDev/FutrixPackage/pkg/auditchain"
	"github.com/FutrixDev/FutrixPackage/pkg/masking"
	"github.com/FutrixDev/FutrixPackage/pkg/protocol"
	"github.com/FutrixDev/FutrixPackage/pkg/riskengine"
)

type BundleResult struct {
	Pass        bool                    `json:"pass"`
	Path        string                  `json:"path"`
	Checks      []CheckResult           `json:"checks"`
	AuditResult auditchain.VerifyResult `json:"auditResult,omitempty"`
}

type CheckResult struct {
	Name    string `json:"name"`
	Pass    bool   `json:"pass"`
	Message string `json:"message,omitempty"`
}

type MaskedQueryExport struct {
	Tool          protocol.ToolName `json:"tool"`
	OK            bool              `json:"ok"`
	DatasourceID  string            `json:"datasourceId"`
	Entity        string            `json:"entity"`
	MaskedColumns []string          `json:"maskedColumns"`
	Rows          []map[string]any  `json:"rows"`
}

func VerifyBundle(path string) (BundleResult, error) {
	path = strings.TrimSpace(path)
	result := BundleResult{Pass: true, Path: path}
	if path == "" {
		return failBundle(result, "bundle_path", "bundle path is required"), nil
	}

	audit, err := auditchain.VerifyFile(filepath.Join(path, "audit-log.jsonl"))
	if err != nil {
		return result, err
	}
	result.AuditResult = audit
	result.add("audit_chain", audit.Pass, audit.Reason)

	maskedOK, msg, err := verifyMaskedQuery(filepath.Join(path, "masked-query-result.json"))
	if err != nil {
		return result, err
	}
	result.add("masked_query_result", maskedOK, msg)

	blockOK, msg, err := verifyRiskBlock(filepath.Join(path, "risk-block-response.json"))
	if err != nil {
		return result, err
	}
	result.add("risk_block_response", blockOK, msg)

	approvalOK, msg, err := verifyApproval(filepath.Join(path, "approval-response.json"))
	if err != nil {
		return result, err
	}
	result.add("approval_response", approvalOK, msg)

	for _, check := range result.Checks {
		if !check.Pass {
			result.Pass = false
			break
		}
	}
	return result, nil
}

func (r *BundleResult) add(name string, pass bool, message string) {
	r.Checks = append(r.Checks, CheckResult{Name: name, Pass: pass, Message: message})
}

func failBundle(result BundleResult, name, message string) BundleResult {
	result.Pass = false
	result.add(name, false, message)
	return result
}

func verifyMaskedQuery(path string) (bool, string, error) {
	var export MaskedQueryExport
	if err := readJSON(path, &export); err != nil {
		return false, "", err
	}
	if !export.OK {
		return false, "masked query export is not ok", nil
	}
	if export.Tool != protocol.ToolExecuteStatement {
		return false, "masked query export must be for execute_statement", nil
	}
	if len(export.MaskedColumns) == 0 {
		return false, "maskedColumns is empty", nil
	}
	if len(export.Rows) == 0 {
		return false, "rows is empty", nil
	}
	for _, col := range export.MaskedColumns {
		for _, row := range export.Rows {
			value, ok := row[col]
			if !ok {
				return false, fmt.Sprintf("masked column %q missing from row", col), nil
			}
			s := fmt.Sprint(value)
			if !masking.IsMaskedValue(s) {
				return false, fmt.Sprintf("masked column %q is not masked", col), nil
			}
		}
	}
	if containsRawPII(export.Rows) {
		return false, "rows still contain raw PII-shaped values", nil
	}
	return true, "", nil
}

func verifyRiskBlock(path string) (bool, string, error) {
	var out protocol.ToolResult
	if err := readJSON(path, &out); err != nil {
		return false, "", err
	}
	if out.OK || out.Error == nil {
		return false, "risk block response must be an error", nil
	}
	if out.Error.RiskAttribution == nil {
		return false, "risk block response lacks riskAttribution", nil
	}
	attr := out.Error.RiskAttribution
	if attr.Action != string(riskengine.ActionBlock) {
		return false, "risk block response action is not block", nil
	}
	assessment := riskengine.NewEngine().Assess("postgresql", "prod-postgres", "DELETE FROM users")
	if assessment.Action != riskengine.ActionBlock {
		return false, "public risk engine did not reproduce block decision", nil
	}
	if attr.RuleID != assessment.RuleID {
		return false, fmt.Sprintf("ruleId mismatch: response=%s engine=%s", attr.RuleID, assessment.RuleID), nil
	}
	return true, "", nil
}

func verifyApproval(path string) (bool, string, error) {
	var out protocol.ToolResult
	if err := readJSON(path, &out); err != nil {
		return false, "", err
	}
	if out.OK || out.ApprovalRequired == nil {
		return false, "approval response must contain approvalRequired", nil
	}
	attr := out.ApprovalRequired.RiskAttribution
	if attr == nil {
		return false, "approval response lacks riskAttribution", nil
	}
	if attr.Action != string(riskengine.ActionWarn) && attr.Action != string(riskengine.ActionRequireApproval) {
		return false, "approval response action is not approval-eligible", nil
	}
	assessment := riskengine.NewEngine().Assess("postgresql", "prod-postgres", "UPDATE users SET status = 'inactive' WHERE id = 1042")
	if assessment.Action != riskengine.ActionWarn {
		return false, "public risk engine did not reproduce warn decision", nil
	}
	if attr.RuleID != assessment.RuleID {
		return false, fmt.Sprintf("ruleId mismatch: response=%s engine=%s", attr.RuleID, assessment.RuleID), nil
	}
	return true, "", nil
}

func readJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

var piiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`),
	regexp.MustCompile(`\+?[0-9][0-9 .()\-]{7,}[0-9]`),
}

func containsRawPII(rows []map[string]any) bool {
	for _, row := range rows {
		for _, value := range row {
			s := fmt.Sprint(value)
			if strings.HasPrefix(s, "masked:") {
				continue
			}
			for _, pattern := range piiPatterns {
				if pattern.MatchString(s) {
					return true
				}
			}
		}
	}
	return false
}

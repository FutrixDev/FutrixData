package console

import (
	"errors"
	"fmt"
	"strings"
)

type ExecutionLimitViolation struct {
	Name      string `json:"name"`
	Requested int    `json:"requested"`
	Maximum   int    `json:"maximum"`
}

type ExecutionLimitError struct {
	Kind           string                    `json:"kind"`
	DatasourceType string                    `json:"datasourceType"`
	Requested      ExecuteBounds             `json:"requestedLimits"`
	Effective      ExecuteBounds             `json:"effectiveLimits"`
	Violations     []ExecutionLimitViolation `json:"violations"`
}

func (e *ExecutionLimitError) Error() string {
	if e == nil {
		return ""
	}
	parts := make([]string, 0, len(e.Violations))
	for _, v := range e.Violations {
		parts = append(parts, fmt.Sprintf("%s %d exceeds policy maximum %d", v.Name, v.Requested, v.Maximum))
	}
	if len(parts) == 0 {
		return "dynamodb execution limits exceed risk policy"
	}
	return "dynamodb execution limits exceed risk policy: " + strings.Join(parts, "; ")
}

func (e *ExecutionLimitError) Details() map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{
		"kind":           e.Kind,
		"datasourceType": e.DatasourceType,
		"requestedLimits": map[string]any{
			"maxReturnedRows":   e.Requested.MaxReturnedRows,
			"maxPages":          e.Requested.MaxPages,
			"maxEvaluatedItems": e.Requested.MaxEvaluatedItems,
			"strictLimits":      e.Requested.StrictLimits,
		},
		"effectiveLimits": map[string]any{
			"maxReturnedRows":   e.Effective.MaxReturnedRows,
			"maxPages":          e.Effective.MaxPages,
			"maxEvaluatedItems": e.Effective.MaxEvaluatedItems,
			"strictLimits":      e.Effective.StrictLimits,
		},
		"violations": e.Violations,
	}
}

func ExecutionLimitErrorFrom(err error) (*ExecutionLimitError, bool) {
	if err == nil {
		return nil, false
	}
	var limitErr *ExecutionLimitError
	if errors.As(err, &limitErr) {
		return limitErr, true
	}
	return nil, false
}

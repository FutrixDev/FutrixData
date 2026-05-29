package toolreg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/riskengine"
)

type redisBatchExecutorService interface {
	ExecuteRedisBatch(context.Context, string, string, []console.RedisBatchOperation, string) (console.RedisBatchResult, error)
}

type redisBatchToolInput struct {
	DatasourceID  string                        `json:"datasourceId"`
	BatchID       string                        `json:"batchId,omitempty"`
	Operations    []console.RedisBatchOperation `json:"operations"`
	ExecutionMode string                        `json:"executionMode,omitempty"`
}

func redisBatchParams() []Param {
	return []Param{
		{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID (must be a Redis datasource)"},
		{Name: "batchId", Type: TypeString, Description: "Optional caller-supplied batch id for audit/search correlation; generated when omitted"},
		{Name: "operations", Type: TypeArray, Required: true, Description: fmt.Sprintf("Redis operations to pipeline; max %d operations. Pipeline mode is not atomic and returns per-operation success/error.", console.MaxRedisBatchOperations), Items: Param{
			Type: TypeObject,
			Properties: []Param{
				{Name: "operationId", Type: TypeString, Description: "Optional caller-supplied operation id for item-level result correlation"},
				{Name: "command", Type: TypeString, Required: true, Description: "Redis command name, e.g. GET, SET, HSET, DEL"},
				{Name: "args", Type: TypeArray, Description: "Redis command arguments as JSON strings; whitespace and binary-safe string values are passed as bulk strings", Items: Param{Type: TypeString}},
			},
		}, MinItems: 1},
		{Name: "executionMode", Type: TypeString, Description: "Execution mode override"},
	}
}

func redisBatchInputFromParams(params map[string]any) (redisBatchToolInput, error) {
	var input redisBatchToolInput
	if err := MapToStruct(params, &input); err != nil {
		return redisBatchToolInput{}, err
	}
	input.DatasourceID = strings.TrimSpace(input.DatasourceID)
	input.BatchID = strings.TrimSpace(input.BatchID)
	input.ExecutionMode = strings.TrimSpace(input.ExecutionMode)
	if input.DatasourceID == "" {
		return redisBatchToolInput{}, fmt.Errorf("datasourceId is required")
	}
	if err := console.ValidateRedisBatchOperations(input.Operations); err != nil {
		return redisBatchToolInput{}, err
	}
	if input.BatchID == "" {
		input.BatchID = newRedisBatchID()
	}
	if params != nil {
		params["batchId"] = input.BatchID
		params["statement"] = console.RedisBatchStatement(input.Operations)
	}
	return input, nil
}

func newRedisBatchID() string {
	return "redis_batch_" + strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000"), ".", "")
}

// AssessRedisBatchApproval evaluates every Redis operation in a structured
// batch. One blocked operation blocks the whole batch; one gated operation
// makes the whole batch wait for approval.
func AssessRedisBatchApproval(ctx context.Context, svc Service, datasourceID string, operations []console.RedisBatchOperation, executionMode string) (ApprovalDecision, error) {
	if strings.TrimSpace(datasourceID) == "" {
		return ApprovalDecision{NeedsApproval: true}, nil
	}
	if err := console.ValidateRedisBatchOperations(operations); err != nil {
		return ApprovalDecision{}, err
	}
	ds, err := svc.GetDatasource(ctx, datasourceID)
	if err != nil {
		return ApprovalDecision{}, err
	}
	if ds.Type != datasource.TypeRedis && ds.Type != datasource.TypeRedisCluster {
		return ApprovalDecision{}, fmt.Errorf("redis datasource required")
	}

	decision := ApprovalDecision{}
	for _, op := range operations {
		assessment, err := svc.AssessRedisCommand(ctx, datasourceID, console.RedisBatchOperationArgs(op), "", executionMode)
		if err != nil {
			return ApprovalDecision{}, err
		}
		if ds.TrustLevel() != datasource.TrustDanger && assessment.Action == riskengine.ActionBlock {
			blockAssessment := assessment
			decision.Blocked = true
			decision.BlockAssessment = &blockAssessment
			return decision, nil
		}
		if assessment.RuleID != "" && assessment.Action != riskengine.ActionAllow && decision.Assessment == nil {
			captured := assessment
			decision.Assessment = &captured
		}
		if riskengine.DecideGate(ds.TrustLevel(), assessment) == riskengine.GateRequireApproval {
			decision.NeedsApproval = true
		}
	}
	return decision, nil
}

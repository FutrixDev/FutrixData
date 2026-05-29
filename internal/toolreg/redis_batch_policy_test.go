package toolreg

import (
	"context"
	"testing"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/riskengine"
)

func TestAssessRedisBatchApprovalChecksEveryOperation(t *testing.T) {
	svc := newPolicyStub(datasource.DataSource{ID: "ds-redis", Type: datasource.TypeRedis})

	decision, err := AssessRedisBatchApproval(context.Background(), svc, "ds-redis", []console.RedisBatchOperation{
		{Command: "GET", Args: []string{"safe:key"}},
		{Command: "SET", Args: []string{"unsafe:key", "value"}},
	}, "")
	if err != nil {
		t.Fatalf("AssessRedisBatchApproval returned error: %v", err)
	}
	if !decision.NeedsApproval {
		t.Fatal("expected batch approval when any operation is warn-risk")
	}
	if decision.Assessment == nil {
		t.Fatal("expected matched risk assessment for the risky operation")
	}
	if decision.Assessment.RuleID != "redis-warn-write" {
		t.Fatalf("rule id = %q, want redis-warn-write", decision.Assessment.RuleID)
	}
}

func TestAssessRedisBatchApprovalBlocksAnyBlockedOperation(t *testing.T) {
	svc := newPolicyStub(datasource.DataSource{ID: "ds-redis", Type: datasource.TypeRedis})

	decision, err := AssessRedisBatchApproval(context.Background(), svc, "ds-redis", []console.RedisBatchOperation{
		{Command: "GET", Args: []string{"safe:key"}},
		{Command: "FLUSHDB"},
	}, "")
	if err != nil {
		t.Fatalf("AssessRedisBatchApproval returned error: %v", err)
	}
	if !decision.Blocked {
		t.Fatal("expected blocked batch when any operation is hard-blocked")
	}
	if decision.BlockAssessment == nil || decision.BlockAssessment.Action != riskengine.ActionBlock {
		t.Fatalf("block assessment = %#v, want block", decision.BlockAssessment)
	}
}

func TestExecuteRedisBatchRejectsOversizedBatchBeforeServiceCall(t *testing.T) {
	def, ok := ByName("execute_redis_batch")
	if !ok {
		t.Fatal("expected execute_redis_batch tool")
	}
	ops := make([]any, 65)
	for i := range ops {
		ops[i] = map[string]any{"command": "GET", "args": []any{"key"}}
	}
	svc := &captureRedisBatchService{policyStubService: newPolicyStub(datasource.DataSource{ID: "ds-redis", Type: datasource.TypeRedis})}

	_, err := def.Call(context.Background(), svc, map[string]any{
		"datasourceId": "ds-redis",
		"operations":   ops,
	})
	if err == nil {
		t.Fatal("expected oversized batch to be rejected")
	}
	if svc.called {
		t.Fatal("execute_redis_batch must not call service after size validation fails")
	}
}

type captureRedisBatchService struct {
	*policyStubService
	called bool
}

func (s *captureRedisBatchService) ExecuteRedisBatch(ctx context.Context, datasourceID, batchID string, operations []console.RedisBatchOperation, executionMode string) (console.RedisBatchResult, error) {
	s.called = true
	return console.RedisBatchResult{BatchID: batchID}, nil
}

package console

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"futrixdata/platform/internal/datasource"
)

const MaxRedisBatchOperations = 64

type RedisBatchOperation struct {
	OperationID string   `json:"operationId,omitempty"`
	Command     string   `json:"command"`
	Args        []string `json:"args,omitempty"`
}

type RedisBatchItemResult struct {
	Index       int    `json:"index"`
	OperationID string `json:"operationId,omitempty"`
	Command     string `json:"command"`
	Success     bool   `json:"success"`
	Result      any    `json:"result,omitempty"`
	Error       string `json:"error,omitempty"`
}

type RedisBatchResult struct {
	BatchID      string                 `json:"batchId"`
	Mode         string                 `json:"mode"`
	Atomic       bool                   `json:"atomic"`
	Total        int                    `json:"total"`
	SuccessCount int                    `json:"successCount"`
	ErrorCount   int                    `json:"errorCount"`
	ElapsedMs    int64                  `json:"elapsedMs"`
	Results      []RedisBatchItemResult `json:"results"`
	Dialect      string                 `json:"dialect,omitempty"`
	Environment  string                 `json:"environment,omitempty"`
}

type RedisBatchExecutor interface {
	ExecuteBatch(ctx context.Context, ds datasource.DataSource, batchID string, operations []RedisBatchOperation) (RedisBatchResult, error)
}

func ValidateRedisBatchOperations(operations []RedisBatchOperation) error {
	if len(operations) == 0 {
		return errors.New("operations must contain at least one Redis command")
	}
	if len(operations) > MaxRedisBatchOperations {
		return fmt.Errorf("operations exceeds maximum batch size %d", MaxRedisBatchOperations)
	}
	for i, op := range operations {
		command := strings.TrimSpace(op.Command)
		if command == "" {
			return fmt.Errorf("operations[%d].command is required", i)
		}
		switch strings.ToLower(command) {
		case "multi", "exec", "discard", "watch", "unwatch":
			return fmt.Errorf("operations[%d].command %q is not supported in Redis pipeline batch; transaction primitives require explicit ownership and timeout semantics", i, command)
		}
	}
	return nil
}

func RedisBatchOperationArgs(op RedisBatchOperation) []string {
	args := make([]string, 0, 1+len(op.Args))
	args = append(args, strings.TrimSpace(op.Command))
	args = append(args, op.Args...)
	return args
}

func RedisBatchOperationStatement(op RedisBatchOperation) string {
	statement, err := RedisCommandStatement(RedisBatchOperationArgs(op))
	if err != nil {
		return strings.ToUpper(strings.TrimSpace(op.Command))
	}
	return statement
}

func RedisBatchStatement(operations []RedisBatchOperation) string {
	lines := make([]string, 0, len(operations))
	for i, op := range operations {
		lines = append(lines, fmt.Sprintf("%d: %s", i, RedisBatchOperationStatement(op)))
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) ExecuteRedisBatch(ctx context.Context, ds datasource.DataSource, batchID string, operations []RedisBatchOperation) (RedisBatchResult, error) {
	done := DatasourceTimingStage(ctx, "manager.redis_batch.validate")
	if err := ValidateRedisBatchOperations(operations); err != nil {
		done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("operations", len(operations)))
		return RedisBatchResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("operations", len(operations)))
	done = DatasourceTimingStage(ctx, "manager.redis_batch.adapter_lookup")
	adapter, err := m.AdapterFor(ds.Type)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return RedisBatchResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	executor, ok := adapter.(RedisBatchExecutor)
	if !ok {
		return RedisBatchResult{}, ErrUnsupported
	}
	// Resolve SecretRef-backed credentials before connecting; mirrors Execute and
	// ExecuteRedisCommand so a batch on a secret-backed Redis datasource does not
	// authenticate with an empty password.
	done = DatasourceTimingStage(ctx, "manager.redis_batch.resolve_datasource")
	ds, err = m.resolveDatasource(ctx, ds)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return RedisBatchResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.redis_batch.adapter_execute")
	result, err := executor.ExecuteBatch(ctx, ds, batchID, operations)
	if err != nil {
		done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("operations", len(operations)))
		return result, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("operations", len(operations)), DatasourceTimingKV("success_count", result.SuccessCount), DatasourceTimingKV("error_count", result.ErrorCount))
	result.Dialect = ds.QueryDialect()
	result.Environment = ds.Environment()
	return result, nil
}

type redisPipelinedClient interface {
	Pipeline() redis.Pipeliner
}

func (r *RedisAdapter) ExecuteBatch(ctx context.Context, ds datasource.DataSource, batchID string, operations []RedisBatchOperation) (RedisBatchResult, error) {
	done := DatasourceTimingStage(ctx, "redis.batch_validate")
	if err := ValidateRedisBatchOperations(operations); err != nil {
		done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("operations", len(operations)))
		return RedisBatchResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("operations", len(operations)))
	done = DatasourceTimingStage(ctx, "redis.client_for")
	client, _, err := r.clientFor(ctx, ds)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return RedisBatchResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	pipelined, ok := client.(redisPipelinedClient)
	if !ok {
		return RedisBatchResult{}, errors.New("redis client does not support pipelining")
	}

	pipe := pipelined.Pipeline()
	cmds := make([]*redis.Cmd, 0, len(operations))
	for _, op := range operations {
		args, err := redisArgsToAny(RedisBatchOperationArgs(op))
		if err != nil {
			return RedisBatchResult{}, err
		}
		cmds = append(cmds, pipe.Do(ctx, args...))
	}

	start := time.Now()
	done = DatasourceTimingStage(ctx, "redis.pipeline_exec")
	_, execErr := pipe.Exec(ctx)
	done(DatasourceTimingKV("status", timingStatus(execErr)), DatasourceTimingKV("operations", len(operations)))
	return buildRedisBatchResult(batchID, operations, cmds, time.Since(start).Milliseconds(), execErr), nil
}

func buildRedisBatchResult(batchID string, operations []RedisBatchOperation, cmds []*redis.Cmd, elapsedMs int64, execErr error) RedisBatchResult {
	result := RedisBatchResult{
		BatchID:   strings.TrimSpace(batchID),
		Mode:      "pipeline",
		Atomic:    false,
		Total:     len(operations),
		ElapsedMs: elapsedMs,
		Results:   make([]RedisBatchItemResult, 0, len(operations)),
	}
	for i, cmd := range cmds {
		item := RedisBatchItemResult{
			Index:       i,
			OperationID: strings.TrimSpace(operations[i].OperationID),
			Command:     strings.ToUpper(strings.TrimSpace(operations[i].Command)),
		}
		if err := cmd.Err(); err != nil {
			item.Error = err.Error()
			result.ErrorCount++
		} else {
			item.Success = true
			item.Result = normalizeRedisResultForJSON(cmd.Val())
			result.SuccessCount++
		}
		result.Results = append(result.Results, item)
	}
	if execErr != nil && result.ErrorCount == 0 {
		message := execErr.Error()
		for i := range result.Results {
			result.Results[i].Success = false
			result.Results[i].Result = nil
			result.Results[i].Error = message
		}
		result.ErrorCount = len(result.Results)
		result.SuccessCount = 0
	}
	return result
}

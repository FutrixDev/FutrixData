package riskengine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
)

type stubProbeProvider struct {
	describeCalls int
	describeNames []string
	describeFn    func(context.Context, datasource.DataSource, string) (console.DescribeResult, error)
	explainCalls  int
	explainFn     func(context.Context, datasource.DataSource, string) (console.ExplainResult, error)
}

func (s *stubProbeProvider) Explain(ctx context.Context, ds datasource.DataSource, statement string) (console.ExplainResult, error) {
	s.explainCalls++
	if s.explainFn != nil {
		return s.explainFn(ctx, ds, statement)
	}
	return console.ExplainResult{}, nil
}

func (s *stubProbeProvider) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (console.DescribeResult, error) {
	s.describeCalls++
	s.describeNames = append(s.describeNames, name)
	if s.describeFn != nil {
		return s.describeFn(ctx, ds, name)
	}
	return console.DescribeResult{}, nil
}

func TestGuard_BlocksHighRisk(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeMySQL,
	}

	err := guard.BeforeExecute(context.Background(), ds, "DROP TABLE users", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected error for DROP TABLE, got nil")
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected BlockedError, got %T: %v", err, err)
	}
	if blocked.Assessment.Action != ActionBlock {
		t.Errorf("Action = %s, want block", blocked.Assessment.Action)
	}
}

func TestGuard_AllowsLowRisk(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeMySQL,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM users", console.ExecuteOptions{})
	if err != nil {
		t.Fatalf("expected nil error for SELECT, got: %v", err)
	}
}

func TestGuardApplyDynamoDBRiskExecutionCapsRecordsClampMetadata(t *testing.T) {
	maxPages := 4
	maxEvaluatedItems := 250
	engine := NewEngine()
	engine.LoadUserRules([]Rule{
		{
			ID:       "tight-dynamodb-budget",
			Scope:    RuleScope{DsTypes: []string{"dynamodb"}},
			Enabled:  true,
			Priority: 200,
			Action:   ActionAllow,
			When:     RuleCondition{Command: []string{"select"}},
			Thresholds: RuleThresholds{
				MaxDynamoDBPages:          &maxPages,
				MaxDynamoDBEvaluatedItems: &maxEvaluatedItems,
			},
		},
	})
	guard := NewGuard(engine)
	opts := console.ExecuteOptions{
		Bounds: console.ExecuteBounds{
			MaxReturnedRows:   100,
			MaxPages:          20,
			MaxEvaluatedItems: 5000,
		},
	}

	if err := guard.ApplyExecuteOptionsCaps(
		context.Background(),
		datasource.DataSource{ID: "ds_dynamodb", Type: datasource.TypeDynamoDB},
		`SELECT * FROM "orders"`,
		&opts,
	); err != nil {
		t.Fatalf("ApplyExecuteOptionsCaps: %v", err)
	}

	if opts.RequestedBounds.MaxPages != 20 || opts.RequestedBounds.MaxEvaluatedItems != 5000 {
		t.Fatalf("RequestedBounds = %#v, want original maxPages=20 maxEvaluatedItems=5000", opts.RequestedBounds)
	}
	if opts.Bounds.MaxPages != maxPages || opts.Bounds.MaxEvaluatedItems != maxEvaluatedItems {
		t.Fatalf("Bounds = %#v, want maxPages=%d maxEvaluatedItems=%d", opts.Bounds, maxPages, maxEvaluatedItems)
	}
	if opts.ClampedLimits["maxPages"] != true || opts.ClampedLimits["maxEvaluatedItems"] != true {
		t.Fatalf("ClampedLimits = %#v, want maxPages and maxEvaluatedItems", opts.ClampedLimits)
	}
}

func TestGuardApplyDynamoDBRiskExecutionCapsRejectsStrictOverPolicy(t *testing.T) {
	maxPages := 4
	maxEvaluatedItems := 250
	engine := NewEngine()
	engine.LoadUserRules([]Rule{
		{
			ID:       "tight-dynamodb-budget",
			Scope:    RuleScope{DsTypes: []string{"dynamodb"}},
			Enabled:  true,
			Priority: 200,
			Action:   ActionAllow,
			When:     RuleCondition{Command: []string{"select"}},
			Thresholds: RuleThresholds{
				MaxDynamoDBPages:          &maxPages,
				MaxDynamoDBEvaluatedItems: &maxEvaluatedItems,
			},
		},
	})
	guard := NewGuard(engine)
	opts := console.ExecuteOptions{
		Bounds: console.ExecuteBounds{
			MaxReturnedRows:   100,
			MaxPages:          20,
			MaxEvaluatedItems: 5000,
			StrictLimits:      true,
		},
	}

	err := guard.ApplyExecuteOptionsCaps(
		context.Background(),
		datasource.DataSource{ID: "ds_dynamodb", Type: datasource.TypeDynamoDB},
		`SELECT * FROM "orders"`,
		&opts,
	)
	if err == nil {
		t.Fatal("expected strict limit error")
	}
	var limitErr *console.ExecutionLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("expected ExecutionLimitError, got %T: %v", err, err)
	}
	if len(limitErr.Violations) != 2 {
		t.Fatalf("violations = %#v, want maxPages and maxEvaluatedItems", limitErr.Violations)
	}
	if opts.RequestedBounds.MaxPages != 20 || opts.RequestedBounds.MaxEvaluatedItems != 5000 {
		t.Fatalf("RequestedBounds = %#v, want original strict request", opts.RequestedBounds)
	}
}

func TestGuard_BlocksWarnInDirectExecution(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeMySQL,
	}

	err := guard.BeforeExecute(context.Background(), ds, "DELETE FROM users WHERE id = 1", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected risk error for DELETE with WHERE, got nil")
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected BlockedError, got %T: %v", err, err)
	}
	if blocked.Assessment.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", blocked.Assessment.Action)
	}
}

func TestGuard_DefaultExecutionCannotBypassBlock(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeMySQL,
	}

	err := guard.BeforeExecute(context.Background(), ds, "DELETE FROM users", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected default execution to remain blocked for DELETE without WHERE")
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected BlockedError, got %T: %v", err, err)
	}
	if blocked.Assessment.Action != ActionBlock {
		t.Fatalf("Action = %s, want block", blocked.Assessment.Action)
	}
}

func TestGuard_NilEngine(t *testing.T) {
	guard := NewGuard(nil)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeMySQL,
	}

	err := guard.BeforeExecute(context.Background(), ds, "DROP TABLE users", console.ExecuteOptions{})
	if err != nil {
		t.Fatalf("expected nil error when engine is nil, got: %v", err)
	}
}

func TestGuard_Redis(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeRedis,
	}

	// FLUSHALL should be blocked
	err := guard.BeforeExecute(context.Background(), ds, "FLUSHALL", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected error for FLUSHALL, got nil")
	}

	// GET should be allowed
	err = guard.BeforeExecute(context.Background(), ds, "GET key", console.ExecuteOptions{})
	if err != nil {
		t.Fatalf("expected nil error for GET, got: %v", err)
	}
}

func TestGuard_Elasticsearch(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeElasticsearch,
	}

	// DELETE should be blocked
	err := guard.BeforeExecute(context.Background(), ds, "DELETE /my-index", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected error for DELETE, got nil")
	}

	// GET should be allowed
	err = guard.BeforeExecute(context.Background(), ds, "GET /logs/_search\n{}", console.ExecuteOptions{})
	if err != nil {
		t.Fatalf("expected nil error for GET _search, got: %v", err)
	}

	err = guard.BeforeExecute(context.Background(), ds, "POST /logs/_search?pretty=true\n{}", console.ExecuteOptions{})
	if err != nil {
		t.Fatalf("expected nil error for POST _search with query string, got: %v", err)
	}
}

func TestGuard_SkipsProbeForNonExplainableSQLRead(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probe := &stubProbeProvider{}
	guard.SetProbeProvider(probe)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeMySQL,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SHOW TABLES", console.ExecuteOptions{})
	if err != nil {
		t.Fatalf("expected nil error for SHOW TABLES, got: %v", err)
	}
	if probe.explainCalls != 0 {
		t.Fatalf("expected explain to be skipped, got %d calls", probe.explainCalls)
	}
}

func TestGuard_SkipsProbeForNonExplainableMongoRead(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probe := &stubProbeProvider{}
	guard.SetProbeProvider(probe)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeMongoDB,
	}

	err := guard.BeforeExecute(context.Background(), ds, `{"action":"getUsers"}`, console.ExecuteOptions{})
	if err != nil {
		t.Fatalf("expected nil error for getUsers, got: %v", err)
	}
	if probe.explainCalls != 0 {
		t.Fatalf("expected explain to be skipped, got %d calls", probe.explainCalls)
	}
}

func TestGuard_RunProbe_ResolvesD1ViewMetadata(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{
				EntityKind:    "view",
				DefinitionSQL: "CREATE VIEW conversion_stats AS SELECT format, COUNT(*) AS total_count FROM conversions GROUP BY format",
			}, nil
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{UsesIndex: false}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}
	ps := ParseStatement(string(ds.Type), ds.ID, "SELECT * FROM conversion_stats LIMIT 50")

	result := guard.runProbe(context.Background(), ds, ps)
	if probeProvider.describeCalls != 1 {
		t.Fatalf("expected describe call, got %d", probeProvider.describeCalls)
	}
	if probeProvider.explainCalls != 1 {
		t.Fatalf("expected explain call, got %d", probeProvider.explainCalls)
	}
	if result.ViewResult == nil {
		t.Fatal("expected ViewResult")
	}
	if result.ViewResult.EntityKind != "view" {
		t.Fatalf("EntityKind = %q, want view", result.ViewResult.EntityKind)
	}
	if result.ViewResult.DefinitionSQL == "" {
		t.Fatal("expected DefinitionSQL")
	}
	if len(result.ViewResult.BaseEntities) != 1 || result.ViewResult.BaseEntities[0] != "conversions" {
		t.Fatalf("BaseEntities = %#v", result.ViewResult.BaseEntities)
	}
}

func TestGuard_RunProbe_DoesNotAttachViewMetadataForTable(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{
				EntityKind:    "table",
				DefinitionSQL: "CREATE TABLE conversions (id INTEGER PRIMARY KEY)",
			}, nil
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{UsesIndex: false}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}
	ps := ParseStatement(string(ds.Type), ds.ID, "SELECT * FROM conversions LIMIT 50")

	result := guard.runProbe(context.Background(), ds, ps)
	if result.ViewResult != nil {
		t.Fatalf("expected nil ViewResult for table, got %#v", result.ViewResult)
	}
}

func TestGuard_RunProbe_CapturesD1ViewParseError(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{
				EntityKind:    "view",
				DefinitionSQL: "CREATE VIEW conversion_stats AS WITH recent AS (SELECT * FROM conversions) SELECT * FROM recent",
			}, nil
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{UsesIndex: true, Stages: []string{"SEARCH conversions USING INTEGER PRIMARY KEY (rowid=?)"}}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}
	ps := ParseStatement(string(ds.Type), ds.ID, "SELECT * FROM conversion_stats LIMIT 50")

	result := guard.runProbe(context.Background(), ds, ps)
	if result.ViewParseErr == nil {
		t.Fatal("expected ViewParseErr")
	}
	if result.ViewResult != nil {
		t.Fatalf("expected nil ViewResult when parse fails, got %#v", result.ViewResult)
	}
}

func TestGuard_RunProbe_DynamoDBDoesNotFallbackForDottedTarget(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(_ context.Context, _ datasource.DataSource, name string) (console.DescribeResult, error) {
			return console.DescribeResult{}, errors.New("table not found")
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeDynamoDB,
	}
	ps := ParseStatement(string(ds.Type), ds.ID, "SELECT * FROM Orders.GenreAndPriceIndex WHERE genre = 'rock'")

	result := guard.runProbe(context.Background(), ds, ps)
	if result.DescribeErr == nil {
		t.Fatal("expected DescribeErr")
	}
	if result.DescribeResult != nil {
		t.Fatalf("expected nil DescribeResult, got %#v", result.DescribeResult)
	}
	if result.DescribeEntity != "" {
		t.Fatalf("DescribeEntity = %q, want empty", result.DescribeEntity)
	}
	if probeProvider.describeCalls != 1 {
		t.Fatalf("describeCalls = %d, want 1", probeProvider.describeCalls)
	}
	if len(probeProvider.describeNames) != 1 || probeProvider.describeNames[0] != "Orders.GenreAndPriceIndex" {
		t.Fatalf("describeNames = %#v", probeProvider.describeNames)
	}
}

func TestGuard_BlocksD1EntityWhenDescribeFails(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{}, errors.New("describe failed")
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{
				UsesIndex: true,
				Stages: []string{
					"CO-ROUTINE conversion_stats",
					"SEARCH conversions USING INTEGER PRIMARY KEY (rowid=?)",
				},
			}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM conversion_stats LIMIT 50", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected error when D1 describe fails, got nil")
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected BlockedError, got %T: %v", err, err)
	}
	if blocked.Assessment.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", blocked.Assessment.Action)
	}
	if !contains(strings.Join(blocked.Assessment.Reasons, " | "), "view definition not verified") {
		t.Fatalf("Reasons = %#v, want view verification reason", blocked.Assessment.Reasons)
	}
}

func TestGuard_ApprovedBypassesProbeWarn(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{UsesIndex: false}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeMySQL,
	}

	// Without Approved, no-index SELECT should be blocked (warn)
	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM users", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected warn error for no-index SELECT without approval")
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected BlockedError, got %T: %v", err, err)
	}
	if blocked.Assessment.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", blocked.Assessment.Action)
	}

	if console.AllowsInteractiveApprovalBypass(console.ExecuteOptions{}) {
		t.Fatal("expected plain execute options to be treated as non-interactive")
	}
}

func TestGuard_AllowsD1TableWhenDescribeFailsButExplainIsIndexed(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{}, errors.New("describe failed")
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{
				UsesIndex: true,
				Stages: []string{
					"SEARCH conversions USING INTEGER PRIMARY KEY (rowid=?)",
				},
			}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM conversions WHERE id = 1", console.ExecuteOptions{})
	if err != nil {
		t.Fatalf("expected indexed D1 table read to stay allowed, got %v", err)
	}
}

func TestGuard_BlocksD1MixedScanAndIndexPlan(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{
				UsesIndex: true,
				Stages: []string{
					"SEARCH users USING INDEX idx_users_id (id=?)",
					"SCAN orders",
				},
			}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM users JOIN orders ON orders.user_id = users.id WHERE users.id = 1", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected mixed D1 scan/index plan to stop, got nil")
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected BlockedError, got %T: %v", err, err)
	}
	if !contains(strings.Join(blocked.Assessment.Reasons, " | "), "execution path not verified") {
		t.Fatalf("Reasons = %#v, want execution-path reason", blocked.Assessment.Reasons)
	}
}

func TestGuard_BlocksFlattenedD1ViewWhenDescribeFails(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{}, errors.New("describe failed")
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{
				UsesIndex: true,
				Stages: []string{
					"SEARCH conversions USING INDEX idx_conversions_format (format=?)",
				},
			}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM conversion_stats LIMIT 50", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected flattened D1 view to stop when metadata lookup fails, got nil")
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected BlockedError, got %T: %v", err, err)
	}
	if !contains(strings.Join(blocked.Assessment.Reasons, " | "), "view definition not verified") {
		t.Fatalf("Reasons = %#v, want view verification reason", blocked.Assessment.Reasons)
	}
}

func TestGuard_AllowsD1TableAliasWhenDescribeFailsButExplainMatchesAlias(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{}, errors.New("describe failed")
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{
				UsesIndex: true,
				Stages: []string{
					"SEARCH c USING INTEGER PRIMARY KEY (rowid=?)",
				},
			}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM conversions c WHERE c.id = 1", console.ExecuteOptions{})
	if err != nil {
		t.Fatalf("expected aliased indexed D1 table read to stay allowed, got %v", err)
	}
}

func TestGuard_AllowsD1BacktickTableAliasWhenDescribeFailsButExplainMatchesAlias(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{}, errors.New("describe failed")
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{
				UsesIndex: true,
				Stages: []string{
					"SEARCH c USING INTEGER PRIMARY KEY (rowid=?)",
				},
			}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM `conversions` c WHERE c.id = 1", console.ExecuteOptions{})
	if err != nil {
		t.Fatalf("expected backtick-quoted aliased indexed D1 table read to stay allowed, got %v", err)
	}
}

func TestGuard_AllowsD1TableWhenDescribeFailsAndExplainIsUnavailable(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{}, errors.New("describe failed")
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{}, console.ErrUnsupported
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM conversions", console.ExecuteOptions{})
	if err != nil {
		t.Fatalf("expected D1 table read with unavailable explain to remain allowed, got %v", err)
	}
}

func TestGuard_BlocksD1ViewWhenExpandedPlanIsExpensive(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{
				EntityKind:    "view",
				DefinitionSQL: "CREATE VIEW conversion_stats AS SELECT format, COUNT(*) AS total_count FROM conversions GROUP BY format",
			}, nil
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{
				UsesIndex: false,
				Stages: []string{
					"CO-ROUTINE conversion_stats",
					"SCAN conversions",
					"USE TEMP B-TREE FOR GROUP BY",
					"SCAN conversion_stats",
				},
			}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM conversion_stats LIMIT 50", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected warn error for expensive D1 view read, got nil")
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected BlockedError, got %T: %v", err, err)
	}
	if blocked.Assessment.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", blocked.Assessment.Action)
	}
	if !contains(strings.Join(blocked.Assessment.Reasons, " | "), "scan on conversions") {
		t.Fatalf("Reasons = %#v, want scan reason", blocked.Assessment.Reasons)
	}
}

func TestGuard_InheritsBaseTableBlockRuleThroughD1View(t *testing.T) {
	engine := NewEngine()
	engine.LoadUserRules([]Rule{
		{
			ID:       "block-conversions-select",
			Scope:    RuleScope{DsTypes: []string{"d1"}, Entity: "conversions"},
			Enabled:  true,
			Priority: 300,
			Action:   ActionBlock,
			Reason:   "conversions reads are blocked",
			When:     RuleCondition{Command: []string{"select"}},
		},
	})
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{
				EntityKind:    "view",
				DefinitionSQL: "CREATE VIEW conversion_stats AS SELECT format, COUNT(*) AS total_count FROM conversions GROUP BY format",
			}, nil
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{
				UsesIndex: true,
				Stages: []string{
					"SEARCH conversions USING INDEX idx_conversions_format (format=?)",
				},
				TotalDocsExamined: 20,
			}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM conversion_stats LIMIT 50", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected block error from inherited base-table rule, got nil")
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected BlockedError, got %T: %v", err, err)
	}
	if blocked.Assessment.Action != ActionBlock {
		t.Fatalf("Action = %s, want block", blocked.Assessment.Action)
	}
	if blocked.Assessment.RuleID != "block-conversions-select" {
		t.Fatalf("RuleID = %q, want block-conversions-select", blocked.Assessment.RuleID)
	}
}

func TestGuard_BlocksD1ViewWhenDefinitionCannotBeVerified(t *testing.T) {
	engine := NewEngine()
	guard := NewGuard(engine)
	probeProvider := &stubProbeProvider{
		describeFn: func(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
			return console.DescribeResult{
				EntityKind:    "view",
				DefinitionSQL: "CREATE VIEW conversion_stats AS WITH recent AS (SELECT * FROM conversions) SELECT * FROM recent",
			}, nil
		},
		explainFn: func(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
			return console.ExplainResult{
				UsesIndex: true,
				Stages: []string{
					"SEARCH conversions USING INTEGER PRIMARY KEY (rowid=?)",
				},
			}, nil
		},
	}
	guard.SetProbeProvider(probeProvider)

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
	}

	err := guard.BeforeExecute(context.Background(), ds, "SELECT * FROM conversion_stats LIMIT 50", console.ExecuteOptions{})
	if err == nil {
		t.Fatal("expected error when D1 view definition cannot be verified, got nil")
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected BlockedError, got %T: %v", err, err)
	}
	if blocked.Assessment.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", blocked.Assessment.Action)
	}
	if !contains(strings.Join(blocked.Assessment.Reasons, " | "), "view definition not verified") {
		t.Fatalf("Reasons = %#v, want view verification reason", blocked.Assessment.Reasons)
	}
}

func TestBlockedError_Message(t *testing.T) {
	err := &BlockedError{
		Assessment: RiskAssessment{
			Action:  ActionBlock,
			Reasons: []string{"destructive DDL"},
		},
	}
	if err.Error() != "statement blocked: destructive DDL" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	err2 := &BlockedError{
		Assessment: RiskAssessment{
			Action:  ActionRequireApproval,
			Reasons: []string{"write operation"},
		},
	}
	if err2.Error() != "statement requires approval: write operation" {
		t.Errorf("unexpected error message: %s", err2.Error())
	}

	err3 := &BlockedError{
		Assessment: RiskAssessment{
			Action:  ActionWarn,
			Reasons: []string{"DELETE"},
		},
	}
	if err3.Error() != "statement stopped for review: DELETE" {
		t.Errorf("unexpected error message: %s", err3.Error())
	}
}

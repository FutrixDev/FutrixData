package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"futrixdata/platform/internal/aichat"
	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/sensitivity"
)

type appExecuteAdapterStub struct {
	result console.QueryResult
}

func (a appExecuteAdapterStub) TestConnection(context.Context, datasource.DataSource) error {
	return nil
}

func (a appExecuteAdapterStub) ListEntities(context.Context, datasource.DataSource, console.ListOptions) ([]string, error) {
	return nil, nil
}

func (a appExecuteAdapterStub) DescribeEntity(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
	return console.DescribeResult{}, nil
}

func (a appExecuteAdapterStub) Execute(context.Context, datasource.DataSource, string, console.ExecuteOptions) (console.QueryResult, error) {
	return a.result, nil
}

func (a appExecuteAdapterStub) Explain(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
	return console.ExplainResult{}, nil
}

func TestAppAIChatTools_ExecuteStatement_KeepsGuiResultRawAndMasksAgentView(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := dsStore.Create(datasource.DataSource{
		Name: "Users MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	if err := sensitivityStore.SetDatasource(sensitivity.DatasourceClassification{
		DatasourceID:   created.ID,
		DatasourceName: created.Name,
		DatasourceType: string(created.Type),
		Entities: map[string]sensitivity.EntityClassification{
			"users": {
				Fields: map[string]sensitivity.FieldClassification{
					"email": {Level: "L4", Category: sensitivity.CategoryPII, Source: sensitivity.SourceManual},
				},
			},
		},
	}); err != nil {
		t.Fatalf("set sensitivity datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(created.Type, appExecuteAdapterStub{
		result: console.QueryResult{
			Columns:      []string{"email"},
			Rows:         []map[string]any{{"email": "user@example.com"}},
			RowCount:     1,
			SourceEntity: "users",
		},
	})

	masking := sensitivity.NewMaskingProcessor(sensitivityStore, []byte("test-local-masking-secret-32bytes"))
	tools := newAppAIChatTools(dsStore, manager, nil, nil, masking, nil, nil, nil, nil)

	result, err := tools.ExecuteStatement(context.Background(), created.ID, "SELECT email FROM users", "", "", 100, true)
	if err != nil {
		t.Fatalf("ExecuteStatement: %v", err)
	}
	got, _ := result.Rows[0]["email"].(string)
	if got != "user@example.com" {
		t.Fatalf("expected gui-facing ai chat result to stay raw, got %q", got)
	}
	if result.AgentView == nil {
		t.Fatal("expected masked agent view to be attached")
	}
	masked, _ := result.AgentView.Rows[0]["email"].(string)
	if !strings.HasPrefix(masked, "masked:") {
		t.Fatalf("expected masked agent-view value, got %q", masked)
	}
}

func TestAppAIChatTools_ExecuteStatement_MasksAgentViewWhenSQLMetadataIsIncomplete(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := dsStore.Create(datasource.DataSource{
		Name: "Contacts MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	sensitivityStore := sensitivity.NewStore(filepath.Join(root, "sensitivity.json"))
	if err := sensitivityStore.SetDatasource(sensitivity.DatasourceClassification{
		DatasourceID:   created.ID,
		DatasourceName: created.Name,
		DatasourceType: string(created.Type),
		Entities: map[string]sensitivity.EntityClassification{
			"fd_crm_contact": {
				Fields: map[string]sensitivity.FieldClassification{
					"email": {Level: "L4", Category: sensitivity.CategoryContact, Source: sensitivity.SourceManual},
					"phone": {Level: "L4", Category: sensitivity.CategoryContact, Source: sensitivity.SourceManual},
				},
			},
		},
	}); err != nil {
		t.Fatalf("set sensitivity datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(created.Type, appExecuteAdapterStub{
		result: console.QueryResult{
			Columns: []string{"contact_id", "email", "phone"},
			Rows: []map[string]any{{
				"contact_id": "1",
				"email":      "contact1@futrix.test",
				"phone":      "+1408000001",
			}},
			ColumnMeta: []console.ResultColumn{
				{Key: "contact_id", Name: "contact_id", Position: 0},
				{Key: "email", Name: "email", Position: 1},
				{Key: "phone", Name: "phone", Position: 2},
			},
			RowValues: [][]any{{"1", "contact1@futrix.test", "+1408000001"}},
			RowCount:  1,
		},
	})

	masking := sensitivity.NewMaskingProcessor(sensitivityStore, []byte("test-local-masking-secret-32bytes"))
	tools := newAppAIChatTools(dsStore, manager, nil, nil, masking, nil, nil, nil, nil)

	result, err := tools.ExecuteStatement(context.Background(), created.ID, "SELECT contact_id, email, phone FROM fd_crm_contact", "", "", 100, true)
	if err != nil {
		t.Fatalf("ExecuteStatement: %v", err)
	}
	if got, _ := result.Rows[0]["email"].(string); got != "contact1@futrix.test" {
		t.Fatalf("expected gui-facing email to stay raw, got %q", got)
	}
	if got, _ := result.Rows[0]["phone"].(string); got != "+1408000001" {
		t.Fatalf("expected gui-facing phone to stay raw, got %q", got)
	}
	if result.AgentView == nil {
		t.Fatal("expected masked agent view to be attached")
	}
	if got, _ := result.AgentView.Rows[0]["email"].(string); !strings.HasPrefix(got, "masked:") {
		t.Fatalf("expected masked agent-view email, got %q", got)
	}
	if got, _ := result.AgentView.Rows[0]["phone"].(string); !strings.HasPrefix(got, "masked:") {
		t.Fatalf("expected masked agent-view phone, got %q", got)
	}
}

func TestApp_ExecuteStatement_LeavesHumanConsoleUnmasked(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := dsStore.Create(datasource.DataSource{
		Name: "Users MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(created.Type, appExecuteAdapterStub{
		result: console.QueryResult{
			Columns:      []string{"email"},
			Rows:         []map[string]any{{"email": "user@example.com"}},
			RowCount:     1,
			SourceEntity: "users",
		},
	})

	app := &App{
		store:   dsStore,
		manager: manager,
	}

	result, err := app.ExecuteStatement(created.ID, "SELECT email FROM users", "", "", 100, "", true, 0, 0, 0)
	if err != nil {
		t.Fatalf("ExecuteStatement: %v", err)
	}
	got, _ := result.Rows[0]["email"].(string)
	if got != "user@example.com" {
		t.Fatalf("expected human console result to stay raw, got %q", got)
	}
}

func TestAppAIChatTools_CreateDatasource_RespectsFreePlanLimit(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	for i := 0; i < 3; i++ {
		if _, err := dsStore.Create(datasource.DataSource{
			Name:     "Seed",
			Type:     datasource.TypeMySQL,
			Host:     "127.0.0.1",
			Port:     3306,
			Username: "root",
			Database: "mysql",
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	tools := newAppAIChatTools(
		dsStore,
		console.NewManager(),
		nil,
		nil,
		nil,
		newAuthStoreWithPlan(t, "free"),
		nil,
		nil,
		nil,
	)

	_, err := tools.CreateDatasource(context.Background(), aichat.DatasourceCreateInput{
		Name:     "Blocked",
		Type:     "mysql",
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "root",
		Database: "mysql",
	})
	if err == nil {
		t.Fatalf("expected ai chat datasource creation to be blocked for free")
	}
	if got := err.Error(); got != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected stable datasource limit error, got %q", got)
	}
}

func TestAppAIChatTools_CreateDatasource_RespectsLoggedOutFreeLimit(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	for i := 0; i < 3; i++ {
		if _, err := dsStore.Create(datasource.DataSource{
			Name:     "Seed",
			Type:     datasource.TypeMySQL,
			Host:     "127.0.0.1",
			Port:     3306,
			Username: "root",
			Database: "mysql",
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	authStore := auth.NewStore(filepath.Join(root, "auth-session.json"))
	if err := authStore.Load(); err != nil {
		t.Fatalf("load auth store: %v", err)
	}
	state := authStore.Current()
	state.Trial = expiredLocalTrial()
	if err := authStore.Save(state); err != nil {
		t.Fatalf("save auth store: %v", err)
	}
	tools := newAppAIChatTools(
		dsStore,
		console.NewManager(),
		nil,
		nil,
		nil,
		authStore,
		nil,
		nil,
		nil,
	)

	_, err := tools.CreateDatasource(context.Background(), aichat.DatasourceCreateInput{
		Name:     "Blocked",
		Type:     "mysql",
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "root",
		Database: "mysql",
	})
	if err == nil {
		t.Fatalf("expected ai chat datasource creation to be blocked for logged-out free use")
	}
	if got := err.Error(); got != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected stable datasource limit error, got %q", got)
	}
}

func TestAppAIChatTools_CreateDatasource_AllowsLoggedOutActiveTrialBeyondFreeLimit(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	for i := 0; i < 3; i++ {
		if _, err := dsStore.Create(datasource.DataSource{
			Name:     "Seed",
			Type:     datasource.TypeMySQL,
			Host:     "127.0.0.1",
			Port:     3306,
			Username: "root",
			Database: "mysql",
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	authStore := auth.NewStore(filepath.Join(root, "auth-session.json"))
	if err := authStore.Load(); err != nil {
		t.Fatalf("load auth store: %v", err)
	}
	state := authStore.Current()
	state.Trial = activeLocalTrial()
	if err := authStore.Save(state); err != nil {
		t.Fatalf("save auth store: %v", err)
	}
	tools := newAppAIChatTools(
		dsStore,
		console.NewManager(),
		nil,
		nil,
		nil,
		authStore,
		nil,
		nil,
		nil,
	)

	created, err := tools.CreateDatasource(context.Background(), aichat.DatasourceCreateInput{
		Name:     "Trial",
		Type:     "mysql",
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "root",
		Database: "mysql",
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}
	if created.Name != "Trial" {
		t.Fatalf("expected trial datasource to be created, got %#v", created)
	}
}

func TestAppAIChatTools_ApprovedFlagUsesInteractiveApprovalPath(t *testing.T) {
	root := t.TempDir()
	dsStore := datasource.NewStore(filepath.Join(root, "datasources.json"))
	created, err := dsStore.Create(datasource.DataSource{
		Name: "Users MySQL",
		Type: datasource.TypeMySQL,
		Host: "127.0.0.1",
		Port: 3306,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(created.Type, appExecuteAdapterStub{})
	manager.SetInterceptor(riskengine.NewGuard(riskengine.NewEngine()))

	tools := newAppAIChatTools(dsStore, manager, nil, nil, nil, nil, nil, nil, nil)

	if _, err := tools.ExecuteStatement(context.Background(), created.ID, "DELETE FROM users", "", "", 100, true); err != nil {
		t.Fatalf("expected approved AI chat execution to proceed, got: %v", err)
	}
}

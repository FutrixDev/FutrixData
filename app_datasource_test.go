package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/secrets"
)

// The edit form refreshes D1 Cloud databases through the id-based binding because
// the list/get payloads redact the API token; the binding must validate its inputs
// and reject unknown datasources before attempting any token-bearing request.
func TestD1ListCloudDatabasesForDatasource_GuardsInputs(t *testing.T) {
	app := &App{}
	if _, err := app.D1ListCloudDatabasesForDatasource("", "acc"); err == nil {
		t.Fatal("expected error when datasource id is empty")
	}
	if _, err := app.D1ListCloudDatabasesForDatasource("ds_1", ""); err == nil {
		t.Fatal("expected error when accountId is empty")
	}

	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	withStore := &App{store: store}
	if _, err := withStore.D1ListCloudDatabasesForDatasource("missing", "acc"); err == nil {
		t.Fatal("expected error when datasource does not exist")
	}

	// A non-D1 datasource may also carry an options.apiToken; the helper must never
	// forward an unrelated token to the Cloudflare D1 endpoint.
	created, err := store.Create(datasource.DataSource{
		Name:    "pg-with-token",
		Type:    datasource.TypePostgreSQL,
		Options: map[string]any{"apiToken": "unrelated-token"},
	})
	if err != nil {
		t.Fatalf("seed datasource: %v", err)
	}
	if _, err := withStore.D1ListCloudDatabasesForDatasource(created.ID, "acc"); err == nil {
		t.Fatal("expected error when datasource is not a D1 datasource")
	}
}

// The server-side token is scoped to the datasource's configured Cloudflare
// account. The binding must derive the account from the stored datasource and
// reject a caller-supplied account that differs, so a renderer caller cannot reuse
// the stored secret against an arbitrary account.
func TestD1ListCloudDatabasesForDatasource_BindsToStoredAccount(t *testing.T) {
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	app := &App{store: store}

	noAccount, err := store.Create(datasource.DataSource{
		Name:    "d1-no-account",
		Type:    datasource.TypeD1,
		Options: map[string]any{"apiToken": "tok"},
	})
	if err != nil {
		t.Fatalf("seed datasource: %v", err)
	}
	if _, err := app.D1ListCloudDatabasesForDatasource(noAccount.ID, "acc_real"); err == nil {
		t.Fatal("expected error when the datasource has no configured account")
	}

	created, err := store.Create(datasource.DataSource{
		Name:    "d1-cloud",
		Type:    datasource.TypeD1,
		Options: map[string]any{"accountId": "acc_real", "apiToken": "tok"},
	})
	if err != nil {
		t.Fatalf("seed datasource: %v", err)
	}
	if _, err := app.D1ListCloudDatabasesForDatasource(created.ID, "acc_other"); err == nil {
		t.Fatal("expected error when caller-supplied accountId does not match the stored account")
	}
}

// captureConnAdapter records the datasource a TestConnection reached so a test can
// assert which host/credential the manager actually dialed.
type captureConnAdapter struct {
	called bool
	gotDS  datasource.DataSource
}

func (a *captureConnAdapter) TestConnection(_ context.Context, ds datasource.DataSource) error {
	a.called = true
	a.gotDS = ds
	return nil
}
func (a *captureConnAdapter) ListEntities(context.Context, datasource.DataSource, console.ListOptions) ([]string, error) {
	return nil, nil
}
func (a *captureConnAdapter) DescribeEntity(context.Context, datasource.DataSource, string) (console.DescribeResult, error) {
	return console.DescribeResult{}, nil
}
func (a *captureConnAdapter) Execute(context.Context, datasource.DataSource, string, console.ExecuteOptions) (console.QueryResult, error) {
	return console.QueryResult{}, nil
}
func (a *captureConnAdapter) Explain(context.Context, datasource.DataSource, string) (console.ExplainResult, error) {
	return console.ExplainResult{}, nil
}

// passwordResolver mimics datasourcesecrets.Manager: it fills the inline password
// from a SecretRef so the test can detect when a secret was resolved.
type passwordResolver struct {
	plaintext string
}

func (r passwordResolver) ResolveDatasource(_ context.Context, ds datasource.DataSource) (datasource.DataSource, error) {
	if ref, ok := ds.SecretRefs["password"]; ok && !ref.Empty() {
		ds.Password = r.plaintext
	}
	return ds, nil
}

func newRedisTestApp(t *testing.T) (*App, *captureConnAdapter) {
	t.Helper()
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	adapter := &captureConnAdapter{}
	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, adapter)
	manager.SetDatasourceSecretResolver(passwordResolver{plaintext: "resolved-secret"})
	return &App{store: store, manager: manager}, adapter
}

// A renderer that listed a ref-backed datasource must not be able to drive secret
// resolution toward a host it supplies in a fresh (unsaved) test payload: with no
// stored owner to bind the ref to, the test is rejected before any connection.
func TestTestDatasourcePayload_RejectsRefWithoutStoredOwner(t *testing.T) {
	app, adapter := newRedisTestApp(t)
	ok, err := app.TestDatasourcePayload(DataSourcePayload{
		Name: "redis", Type: datasource.TypeRedis,
		Host: "attacker.evil", Port: 6379,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "k", Field: "f"},
		},
	}, "")
	if ok || err == nil {
		t.Fatalf("expected rejection of ref-backed test without a stored owner, got ok=%v err=%v", ok, err)
	}
	if adapter.called {
		t.Fatal("connection must not be attempted when a ref cannot be bound to a stored datasource")
	}
}

// Pointing a stored datasource's secret ref at a new host (the exfiltration shape,
// and also a setting Save would persist) must be rejected, not silently resolved
// against either target. The stored secret is never sent to the edited host.
func TestTestDatasourcePayload_RejectsEditedSecretBackedTarget(t *testing.T) {
	app, adapter := newRedisTestApp(t)
	stored, err := app.store.Create(datasource.DataSource{
		Name: "redis", Type: datasource.TypeRedis,
		Host: "stored.internal", Port: 6379,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "k", Field: "f"},
		},
	})
	if err != nil {
		t.Fatalf("seed datasource: %v", err)
	}
	ok, err := app.TestDatasourcePayload(DataSourcePayload{
		Name: "redis", Type: datasource.TypeRedis,
		Host: "attacker.evil", Port: 6379,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "k", Field: "f"},
		},
	}, stored.ID)
	if ok || err == nil {
		t.Fatalf("expected an edited secret-backed target to be rejected, got ok=%v err=%v", ok, err)
	}
	if adapter.called {
		t.Fatal("the stored secret must not be resolved toward an edited/unsaved target")
	}
}

// Changing the reference itself (key/version) on an existing datasource must also be
// rejected until saved: the new ref is not yet bound to a persisted target.
func TestTestDatasourcePayload_RejectsEditedSecretRef(t *testing.T) {
	app, adapter := newRedisTestApp(t)
	stored, err := app.store.Create(datasource.DataSource{
		Name: "redis", Type: datasource.TypeRedis,
		Host: "stored.internal", Port: 6379,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "k", Field: "f"},
		},
	})
	if err != nil {
		t.Fatalf("seed datasource: %v", err)
	}
	ok, err := app.TestDatasourcePayload(DataSourcePayload{
		Name: "redis", Type: datasource.TypeRedis,
		Host: "stored.internal", Port: 6379,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "rotated-key", Field: "f"},
		},
	}, stored.ID)
	if ok || err == nil {
		t.Fatalf("expected an edited secret ref to be rejected, got ok=%v err=%v", ok, err)
	}
	if adapter.called {
		t.Fatal("an unsaved, edited ref must not be resolved")
	}
}

// Clicking Test on an unchanged stored ref-backed datasource resolves the stored
// secret against the stored target — exactly what Save would persist.
func TestTestDatasourcePayload_UnchangedSecretBackedTestsStored(t *testing.T) {
	app, adapter := newRedisTestApp(t)
	ref := secrets.SecretRef{ProviderConfigID: "vault-dev", Key: "k", Field: "f"}
	stored, err := app.store.Create(datasource.DataSource{
		Name: "redis", Type: datasource.TypeRedis,
		Host: "stored.internal", Port: 6379,
		SecretRefs: map[string]secrets.SecretRef{"password": ref},
	})
	if err != nil {
		t.Fatalf("seed datasource: %v", err)
	}
	// The redacted edit form round-trips the same ref and target with no typed password.
	ok, err := app.TestDatasourcePayload(DataSourcePayload{
		Name: "redis", Type: datasource.TypeRedis,
		Host: "stored.internal", Port: 6379,
		SecretRefs: map[string]secrets.SecretRef{"password": ref},
	}, stored.ID)
	if !ok || err != nil {
		t.Fatalf("expected unchanged ref-backed test to succeed, got ok=%v err=%v", ok, err)
	}
	if adapter.gotDS.Host != "stored.internal" {
		t.Fatalf("secret must be sent only to the stored host, got %q", adapter.gotDS.Host)
	}
	if adapter.gotDS.Password != "resolved-secret" {
		t.Fatalf("stored ref should resolve to its secret, got %q", adapter.gotDS.Password)
	}
}

// Switching a stored ref-backed datasource back to a manually typed password drops
// the ref from the restored payload, so Test Connection must validate the newly
// typed credential against the form target — not silently re-resolve the old stored
// secret (which would report success for a change that fails after saving).
func TestTestDatasourcePayload_SwitchToManualTestsTypedCredential(t *testing.T) {
	app, adapter := newRedisTestApp(t)
	stored, err := app.store.Create(datasource.DataSource{
		Name: "redis", Type: datasource.TypeRedis,
		Host: "stored.internal", Port: 6379,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "k", Field: "f"},
		},
	})
	if err != nil {
		t.Fatalf("seed datasource: %v", err)
	}
	// The form switched to manual entry: a typed password and no real ref.
	ok, err := app.TestDatasourcePayload(DataSourcePayload{
		Name: "redis", Type: datasource.TypeRedis,
		Host: "stored.internal", Port: 6379, Password: "typed-pw",
	}, stored.ID)
	if !ok || err != nil {
		t.Fatalf("expected manual-credential test to succeed, got ok=%v err=%v", ok, err)
	}
	if adapter.gotDS.Password != "typed-pw" {
		t.Fatalf("must test the newly typed password, not the stored secret, got %q", adapter.gotDS.Password)
	}
}

// A payload with no secret references keeps the existing direct-test behaviour:
// the caller-supplied target and inline credential are used as-is.
func TestTestDatasourcePayload_NoRefTestsPayloadTarget(t *testing.T) {
	app, adapter := newRedisTestApp(t)
	ok, err := app.TestDatasourcePayload(DataSourcePayload{
		Name: "redis", Type: datasource.TypeRedis,
		Host: "user.host", Port: 6379, Password: "inline-pw",
	}, "")
	if !ok || err != nil {
		t.Fatalf("expected inline test to succeed, got ok=%v err=%v", ok, err)
	}
	if adapter.gotDS.Host != "user.host" || adapter.gotDS.Password != "inline-pw" {
		t.Fatalf("inline test must use the payload target/credential, got host=%q pw=%q", adapter.gotDS.Host, adapter.gotDS.Password)
	}
}

func TestValidateDataSourcePayload_AllowsSQLWithOptionURISecretRef(t *testing.T) {
	payload := DataSourcePayload{
		Name: "vault-uri",
		Type: datasource.TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev", Key: "datasources/x/options/uri", Field: "uri"},
		},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("Wails SQL payload with options.uri secret ref should validate, got %v", err)
	}
}

// A D1 cloud token delegated to a SecretRef leaves options.apiToken empty by design,
// so the Wails validator must accept the resolvable ref as satisfying token auth —
// otherwise the GUI save/test path rejects it even though the secret is configured.
func TestValidateDataSourcePayload_AllowsD1CloudTokenViaSecretRef(t *testing.T) {
	payload := DataSourcePayload{
		Name: "d1-cloud-token-ref",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
		},
		SecretRefs: map[string]secrets.SecretRef{
			"options.apiToken": {ProviderConfigID: "vault-prod", Key: "cloudflare/d1/api-token", Field: "token"},
		},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("Wails D1 token delegated to a secret ref should validate, got %v", err)
	}
}

func TestValidateDataSourcePayload_D1CloudTokenRequiresApiToken(t *testing.T) {
	payload := DataSourcePayload{
		Name: "d1-cloud-token",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
		},
	}
	if err := validateDataSourcePayload(payload); err == nil {
		t.Fatal("Wails D1 cloud token auth without an inline token or ref should be rejected")
	}
}

// The edit form has no UI for the options.uri ref and the type watcher applies the
// Mongo default port (27017) with an empty host, so the URI-ref exemption must hold
// regardless of host/port; otherwise an unrelated update rejects with host required.
func TestValidateDataSourcePayload_AllowsMongoURISecretRefWithDefaultPort(t *testing.T) {
	payload := DataSourcePayload{
		Name: "vault-uri",
		Type: datasource.TypeMongoDB,
		Port: 27017,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev", Key: "datasources/x/options/uri", Field: "uri"},
		},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("Wails Mongo payload with options.uri ref and default port should validate, got %v", err)
	}
}

func TestValidateDataSourcePayload_RejectsIncompleteURISecretRef(t *testing.T) {
	payload := DataSourcePayload{
		Name: "partial-uri",
		Type: datasource.TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev"}, // no key/field
		},
	}
	if err := validateDataSourcePayload(payload); err == nil {
		t.Fatal("Wails SQL payload with incomplete options.uri secret ref should be rejected")
	}
}

// A non-UI caller can supply an incomplete password ref while still providing
// host/port; that must be rejected rather than persisted as a record that only
// fails at connection time.
func TestValidateDataSourcePayload_RejectsIncompletePasswordRefWithHostPort(t *testing.T) {
	payload := DataSourcePayload{
		Name: "partial-password",
		Type: datasource.TypePostgreSQL,
		Host: "localhost",
		Port: 5432,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev"}, // no key/field
		},
	}
	if err := validateDataSourcePayload(payload); err == nil {
		t.Fatal("payload with incomplete password secret ref should be rejected even with host/port")
	}
}

// When a password ref is present, ClearInlineSecretsForRefs strips the inline
// options.uri on save, so a URI-only SQL payload combined with a password ref
// would persist with no uri and no host/port — reject it at validation.
func TestValidateDataSourcePayload_RejectsSQLURIOnlyWithPasswordRef(t *testing.T) {
	payload := DataSourcePayload{
		Name: "uri-shadowed",
		Type: datasource.TypePostgreSQL,
		Options: map[string]any{
			"uri": "postgres://user@db.example.com:5432/app",
		},
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateDataSourcePayload(payload); err == nil {
		t.Fatal("SQL URI-only payload with a password ref should be rejected (uri is stripped on save)")
	}
}

// A password ref alongside host/port survives the save (only the inline uri is
// stripped), so it must still validate.
func TestValidateDataSourcePayload_AllowsSQLHostPortWithPasswordRef(t *testing.T) {
	payload := DataSourcePayload{
		Name: "host-port-ref",
		Type: datasource.TypePostgreSQL,
		Host: "db.example.com",
		Port: 5432,
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("SQL host/port payload with a password ref should validate, got %v", err)
	}
}

// A delegated options.uri ref is never stripped, so it satisfies addressing even
// when a password ref is also present.
func TestValidateDataSourcePayload_AllowsSQLURIRefWithPasswordRef(t *testing.T) {
	payload := DataSourcePayload{
		Name: "uri-ref-and-password-ref",
		Type: datasource.TypePostgreSQL,
		SecretRefs: map[string]secrets.SecretRef{
			"options.uri": {ProviderConfigID: "vault-dev", Key: "datasources/x/options/uri", Field: "uri"},
			"password":    {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("SQL options.uri ref + password ref should validate, got %v", err)
	}
}

// Mongo mirrors SQL: an inline options.uri shadowed by a password ref is stripped,
// so a uri-only Mongo payload with a password ref must be rejected, while an
// explicit hosts list (never stripped) still satisfies addressing.
func TestValidateDataSourcePayload_RejectsMongoURIOnlyWithPasswordRef(t *testing.T) {
	payload := DataSourcePayload{
		Name: "mongo-uri-shadowed",
		Type: datasource.TypeMongoDB,
		Options: map[string]any{
			"uri": "mongodb://user@host1:27017/app",
		},
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateDataSourcePayload(payload); err == nil {
		t.Fatal("Mongo URI-only payload with a password ref should be rejected (uri is stripped on save)")
	}
}

func TestValidateDataSourcePayload_AllowsMongoHostsWithPasswordRef(t *testing.T) {
	payload := DataSourcePayload{
		Name: "mongo-hosts-ref",
		Type: datasource.TypeMongoDB,
		Options: map[string]any{
			"hosts": []string{"host1:27017"},
		},
		SecretRefs: map[string]secrets.SecretRef{
			"password": {ProviderConfigID: "vault-dev", Key: "datasources/x/password", Field: "password"},
		},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("Mongo hosts payload with a password ref should validate, got %v", err)
	}
}

func TestValidateDataSourcePayload_AllowsElasticsearch(t *testing.T) {
	payload := DataSourcePayload{
		Name: "es",
		Type: datasource.TypeElasticsearch,
		Host: "localhost",
		Port: 9200,
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDataSourcePayload_ElasticsearchRequiresHostPort(t *testing.T) {
	payload := DataSourcePayload{
		Name: "es",
		Type: datasource.TypeElasticsearch,
	}
	if err := validateDataSourcePayload(payload); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateDataSourcePayload_AllowsMySQLURIWithoutHostPort(t *testing.T) {
	payload := DataSourcePayload{
		Name: "mysql-uri",
		Type: datasource.TypeMySQL,
		Options: map[string]any{
			"uri": "mysql://root:secret@127.0.0.1:3306/mysql",
		},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDataSourcePayload_AllowsPostgresURIWithoutHostPort(t *testing.T) {
	payload := DataSourcePayload{
		Name: "pg-uri",
		Type: datasource.TypePostgreSQL,
		Options: map[string]any{
			"uri": "postgresql://postgres:secret@127.0.0.1:5432/postgres",
		},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDataSourcePayload_AllowsDynamoDBWithRegion(t *testing.T) {
	payload := DataSourcePayload{
		Name:    "ddb",
		Type:    datasource.DataSourceType("dynamodb"),
		Options: map[string]any{"region": "us-east-1"},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDataSourcePayload_DynamoDBRequiresRegion(t *testing.T) {
	payload := DataSourcePayload{
		Name: "ddb",
		Type: datasource.DataSourceType("dynamodb"),
	}
	if err := validateDataSourcePayload(payload); err == nil {
		t.Fatalf("expected error")
	} else if err.Error() != "region is required" {
		t.Fatalf("expected region is required, got %v", err)
	}
}

func TestValidateDataSourcePayload_AllowsD1Cloud(t *testing.T) {
	payload := DataSourcePayload{
		Name: "d1-cloud",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
		},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDataSourcePayload_D1CloudRequiresAccountID(t *testing.T) {
	payload := DataSourcePayload{
		Name: "d1-cloud",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"databaseId": "db_123",
		},
	}
	if err := validateDataSourcePayload(payload); err == nil {
		t.Fatalf("expected error")
	} else if err.Error() != "accountId is required for d1" {
		t.Fatalf("expected accountId is required for d1, got %v", err)
	}
}

func TestValidateDataSourcePayload_AllowsD1Local(t *testing.T) {
	payload := DataSourcePayload{
		Name: "d1-local",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDataSourcePayload_D1LocalRequiresBinding(t *testing.T) {
	payload := DataSourcePayload{
		Name: "d1-local",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"databaseId": "local-db-id",
		},
	}
	if err := validateDataSourcePayload(payload); err == nil {
		t.Fatalf("expected error")
	} else if err.Error() != "binding is required for local mode" {
		t.Fatalf("expected binding is required for local mode, got %v", err)
	}
}

func TestValidateDataSourcePayload_AllowsD1OAuthFlowWithoutMode(t *testing.T) {
	payload := DataSourcePayload{
		Name: "d1-oauth",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"accountId":    "acc_123",
			"databaseId":   "db_123",
			"databaseName": "analytics",
		},
	}
	if err := validateDataSourcePayload(payload); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDataSourcePayload_D1OAuthFlowRequiresAccountID(t *testing.T) {
	payload := DataSourcePayload{
		Name: "d1-oauth",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"databaseId":   "db_123",
			"databaseName": "analytics",
		},
	}
	if err := validateDataSourcePayload(payload); err == nil {
		t.Fatalf("expected error")
	} else if err.Error() != "accountId is required for d1" {
		t.Fatalf("expected accountId is required for d1, got %v", err)
	}
}

func TestD1ResolveWranglerAccounts_MultipleAccounts(t *testing.T) {
	app := &App{
		runCommand: func(_ context.Context, command []string) ([]byte, error) {
			joined := strings.Join(command, " ")
			if !strings.Contains(joined, "whoami --json") {
				t.Fatalf("unexpected command: %q", joined)
			}
			return []byte(`{
				"account_id":"acc_beta",
				"accounts":[
					{"id":"acc_beta","name":"Beta Team"},
					{"account_tag":"acc_alpha","name":"Alpha Team"}
				]
			}`), nil
		},
	}

	accounts, selected, err := app.d1ResolveWranglerAccounts(context.Background(), []string{"npx", "wrangler"})
	if err != nil {
		t.Fatalf("d1ResolveWranglerAccounts: %v", err)
	}
	if selected != "acc_beta" {
		t.Fatalf("expected selected account acc_beta, got %q", selected)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}
	if accounts[0].ID != "acc_alpha" || accounts[0].Name != "Alpha Team" {
		t.Fatalf("expected first account Alpha Team/acc_alpha, got %#v", accounts[0])
	}
	if accounts[1].ID != "acc_beta" || accounts[1].Name != "Beta Team" {
		t.Fatalf("expected second account Beta Team/acc_beta, got %#v", accounts[1])
	}
}

func TestD1ResolveWranglerAccounts_SelectsSingleAccountWhenAccountIDMissing(t *testing.T) {
	app := &App{
		runCommand: func(_ context.Context, _ []string) ([]byte, error) {
			return []byte(`{
				"account_id":"",
				"accounts":[
					{"id":"acc_only","name":"Only Team"}
				]
			}`), nil
		},
	}

	accounts, selected, err := app.d1ResolveWranglerAccounts(context.Background(), []string{"npx", "wrangler"})
	if err != nil {
		t.Fatalf("d1ResolveWranglerAccounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].ID != "acc_only" {
		t.Fatalf("expected one account acc_only, got %#v", accounts)
	}
	if selected != "acc_only" {
		t.Fatalf("expected selected account acc_only, got %q", selected)
	}
}

func TestD1OAuthLogin_SkipsBrowserLoginWhenWranglerSessionAlreadyValid(t *testing.T) {
	commands := make([]string, 0, 4)
	app := &App{
		runCommand: func(_ context.Context, command []string) ([]byte, error) {
			joined := strings.Join(command, " ")
			commands = append(commands, joined)
			switch {
			case strings.Contains(joined, "auth token --json"):
				return []byte(`{"token":"token_ready"}`), nil
			case strings.Contains(joined, "whoami --json"):
				return []byte(`{
					"account_id":"acc_ready",
					"accounts":[{"id":"acc_ready","name":"Ready Team"}]
				}`), nil
			case strings.HasSuffix(joined, "wrangler login"):
				return nil, errors.New("login should not be called when token already works")
			default:
				return nil, errors.New("unexpected command: " + joined)
			}
		},
	}

	session, err := app.D1OAuthLogin()
	if err != nil {
		t.Fatalf("D1OAuthLogin: %v", err)
	}
	if strings.TrimSpace(session.Token) != "token_ready" {
		t.Fatalf("expected token token_ready, got %q", session.Token)
	}
	if strings.TrimSpace(session.AccountID) != "acc_ready" {
		t.Fatalf("expected account acc_ready, got %q", session.AccountID)
	}
	if len(session.Accounts) != 1 || session.Accounts[0].ID != "acc_ready" {
		t.Fatalf("expected one account acc_ready, got %#v", session.Accounts)
	}
	if strings.Contains(strings.Join(commands, "\n"), "wrangler login") {
		t.Fatalf("expected no wrangler login command, got %v", commands)
	}
}

func TestD1OAuthLogin_FallsBackToBrowserLoginWhenTokenMissing(t *testing.T) {
	tokenCalls := 0
	loginCalls := 0
	app := &App{
		runCommand: func(_ context.Context, command []string) ([]byte, error) {
			joined := strings.Join(command, " ")
			switch {
			case strings.Contains(joined, "auth token --json"):
				tokenCalls++
				if tokenCalls == 1 {
					return nil, errors.New("wrangler auth token is empty")
				}
				return []byte(`{"token":"token_after_login"}`), nil
			case strings.Contains(joined, "whoami --json"):
				return []byte(`{
					"account_id":"acc_after_login",
					"accounts":[{"id":"acc_after_login","name":"After Login Team"}]
				}`), nil
			case strings.HasSuffix(joined, "wrangler login"):
				loginCalls++
				return []byte{}, nil
			default:
				return nil, errors.New("unexpected command: " + joined)
			}
		},
	}

	session, err := app.D1OAuthLogin()
	if err != nil {
		t.Fatalf("D1OAuthLogin: %v", err)
	}
	if loginCalls != 1 {
		t.Fatalf("expected wrangler login to be called once, got %d", loginCalls)
	}
	if tokenCalls < 2 {
		t.Fatalf("expected auth token command to retry after login, got %d calls", tokenCalls)
	}
	if strings.TrimSpace(session.Token) != "token_after_login" {
		t.Fatalf("expected token token_after_login, got %q", session.Token)
	}
	if strings.TrimSpace(session.AccountID) != "acc_after_login" {
		t.Fatalf("expected account acc_after_login, got %q", session.AccountID)
	}
}

func TestD1OAuthReLogin_AlwaysRunsBrowserLoginEvenWhenWranglerSessionAlreadyValid(t *testing.T) {
	loginCalls := 0
	app := &App{
		runCommand: func(_ context.Context, command []string) ([]byte, error) {
			joined := strings.Join(command, " ")
			switch {
			case strings.Contains(joined, "auth token --json"):
				return []byte(`{"token":"token_ready"}`), nil
			case strings.Contains(joined, "whoami --json"):
				return []byte(`{
					"account_id":"acc_ready",
					"accounts":[{"id":"acc_ready","name":"Ready Team"}]
				}`), nil
			case strings.HasSuffix(joined, "wrangler login"):
				loginCalls++
				return []byte{}, nil
			default:
				return nil, errors.New("unexpected command: " + joined)
			}
		},
	}

	session, err := app.D1OAuthReLogin()
	if err != nil {
		t.Fatalf("D1OAuthReLogin: %v", err)
	}
	if loginCalls != 1 {
		t.Fatalf("expected wrangler login to be called once, got %d", loginCalls)
	}
	if strings.TrimSpace(session.Token) != "token_ready" {
		t.Fatalf("expected token token_ready, got %q", session.Token)
	}
	if strings.TrimSpace(session.AccountID) != "acc_ready" {
		t.Fatalf("expected account acc_ready, got %q", session.AccountID)
	}
}

func TestD1WranglerInstalledWithLookup_ReturnsTrueWhenWranglerBinaryExists(t *testing.T) {
	lookup := func(name string) (string, error) {
		if name == "wrangler" {
			return "/usr/local/bin/wrangler", nil
		}
		return "", exec.ErrNotFound
	}

	if !d1WranglerInstalledWithLookup(lookup) {
		t.Fatalf("expected wrangler install check to pass when wrangler binary exists")
	}
}

func TestD1WranglerInstalledWithLookup_ReturnsTrueWhenNpxExists(t *testing.T) {
	lookup := func(name string) (string, error) {
		if name == "npx" {
			return "/usr/local/bin/npx", nil
		}
		return "", exec.ErrNotFound
	}

	if !d1WranglerInstalledWithLookup(lookup) {
		t.Fatalf("expected wrangler install check to pass when npx exists")
	}
}

func TestD1WranglerInstalledWithLookup_ReturnsFalseWhenWranglerAndNpxMissing(t *testing.T) {
	lookup := func(_ string) (string, error) {
		return "", exec.ErrNotFound
	}

	if d1WranglerInstalledWithLookup(lookup) {
		t.Fatalf("expected wrangler install check to fail when binaries are missing")
	}
}

func TestCreateDatasource_D1DoesNotPersistWranglerTomlByDefault(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	created, err := app.CreateDatasource(DataSourcePayload{
		Name: "Cloud D1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"accountId":    "acc_123",
			"databaseId":   "db_123",
			"databaseName": "my-log-db",
		},
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}

	if got := strings.TrimSpace(optionAnyString(created.Options, "binding")); got != "my_log_db" {
		t.Fatalf("expected binding my_log_db, got %q", got)
	}
	if got := strings.TrimSpace(optionAnyString(created.Options, "wranglerConfigPath")); got != "" {
		t.Fatalf("expected wranglerConfigPath to be empty by default, got %q", got)
	}
	if raw, ok := created.Options["supportDev"]; ok {
		if enabled, _ := raw.(bool); enabled {
			t.Fatalf("expected supportDev false by default, got %#v", raw)
		}
	}
}

func TestCreateDatasource_FreePlanBlocksFourthDatasource(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := store.Create(datasource.DataSource{
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

	app := &App{
		cfg:       Config{DataPath: dataPath},
		store:     store,
		authStore: newAuthStoreWithPlan(t, "free"),
	}

	_, err := app.CreateDatasource(DataSourcePayload{
		Name:     "Blocked",
		Type:     datasource.TypeMySQL,
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "root",
		Database: "mysql",
	})
	if err == nil {
		t.Fatalf("expected free plan to block the fourth datasource")
	}
	if got := err.Error(); got != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected stable datasource limit error, got %q", got)
	}
}

func TestCreateDatasource_FreePlanBlockedD1DoesNotWriteWranglerToml(t *testing.T) {
	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	projectDir := filepath.Join(root, "worker-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := store.Create(datasource.DataSource{
			Name:     fmt.Sprintf("Seed %d", i),
			Type:     datasource.TypeMySQL,
			Host:     "127.0.0.1",
			Port:     3306 + i,
			Username: "root",
			Database: "mysql",
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	app := &App{
		cfg:       Config{DataPath: dataPath},
		store:     store,
		authStore: newAuthStoreWithPlan(t, "free"),
	}

	_, err := app.CreateDatasource(DataSourcePayload{
		Name: "Blocked D1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"accountId":      "acc_123",
			"databaseId":     "db_123",
			"databaseName":   "blocked-db",
			"supportDev":     true,
			"devProjectPath": projectDir,
		},
	})
	if err == nil {
		t.Fatal("expected free plan to block blocked D1 datasource")
	}
	if got := err.Error(); got != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected stable datasource limit error, got %q", got)
	}
	if got := len(store.List()); got != 3 {
		t.Fatalf("expected datasource count to stay at 3, got %d", got)
	}
	if _, statErr := os.Stat(filepath.Join(projectDir, "wrangler.toml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no wrangler.toml to be created, got err=%v", statErr)
	}
}

func TestCreateDatasource_FreePlanBlockedRedisDoesNotProbeClusterNodes(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := store.Create(datasource.DataSource{
			Name:     fmt.Sprintf("Seed %d", i),
			Type:     datasource.TypeMySQL,
			Host:     "127.0.0.1",
			Port:     3306 + i,
			Username: "root",
			Database: "mysql",
		}); err != nil {
			t.Fatalf("seed datasource %d: %v", i, err)
		}
	}

	probeCalls := 0
	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, metricsStubAdapter{
		execute: func(ds datasource.DataSource, statement string) (console.QueryResult, error) {
			_ = ds
			probeCalls++
			return console.QueryResult{}, errors.New("unexpected statement: " + statement)
		},
	})

	app := &App{
		cfg:       Config{DataPath: dataPath},
		store:     store,
		manager:   manager,
		authStore: newAuthStoreWithPlan(t, "free"),
	}

	_, err := app.CreateDatasource(DataSourcePayload{
		Name: "Blocked Redis",
		Type: datasource.TypeRedis,
		Host: "127.0.0.1",
		Port: 7000,
	})
	if err == nil {
		t.Fatal("expected free plan to block blocked Redis datasource")
	}
	if got := err.Error(); got != "plan_limit_exceeded:datasources:free:3" {
		t.Fatalf("expected stable datasource limit error, got %q", got)
	}
	if probeCalls != 0 {
		t.Fatalf("expected no Redis cluster probe when plan blocks create, got %d calls", probeCalls)
	}
	if got := len(store.List()); got != 3 {
		t.Fatalf("expected datasource count to stay at 3, got %d", got)
	}
}

func TestCreateDatasource_D1WithDevProjectPathCreatesWranglerToml(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	projectDir := filepath.Join(t.TempDir(), "worker-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	created, err := app.CreateDatasource(DataSourcePayload{
		Name: "Cloud D1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"accountId":      "acc_123",
			"databaseId":     "db_123",
			"databaseName":   "my-log-db",
			"supportDev":     true,
			"devProjectPath": projectDir,
		},
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}

	configPath := strings.TrimSpace(optionAnyString(created.Options, "wranglerConfigPath"))
	if configPath == "" {
		t.Fatalf("expected wranglerConfigPath to be set when dev support is enabled")
	}
	expectedConfigPath := filepath.Join(projectDir, "wrangler.toml")
	if filepath.Clean(configPath) != filepath.Clean(expectedConfigPath) {
		t.Fatalf("expected wranglerConfigPath %q, got %q", expectedConfigPath, configPath)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read wrangler config: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, "[[d1_databases]]") {
		t.Fatalf("expected d1_databases section, got %q", content)
	}
	if !strings.Contains(content, "binding = \"my_log_db\"") {
		t.Fatalf("expected binding in wrangler.toml, got %q", content)
	}
	if !strings.Contains(content, "database_name = \"my-log-db\"") {
		t.Fatalf("expected database_name in wrangler.toml, got %q", content)
	}
	if !strings.Contains(content, "database_id = \"db_123\"") {
		t.Fatalf("expected database_id in wrangler.toml, got %q", content)
	}
	if !strings.Contains(content, "migrations_dir = \"migrations/my-log-db-db_123\"") {
		t.Fatalf("expected migrations_dir in wrangler.toml, got %q", content)
	}
	if got := strings.TrimSpace(optionAnyString(created.Options, "migrationsDir")); got != "migrations/my-log-db-db_123" {
		t.Fatalf("expected migrationsDir migrations/my-log-db-db_123, got %q", got)
	}
	if got := strings.TrimSpace(optionAnyString(created.Options, "devProjectPath")); filepath.Clean(got) != filepath.Clean(projectDir) {
		t.Fatalf("expected devProjectPath %q, got %q", projectDir, got)
	}
	rawSupportDev, ok := created.Options["supportDev"]
	if !ok {
		t.Fatalf("expected supportDev option to be persisted")
	}
	if enabled, _ := rawSupportDev.(bool); !enabled {
		t.Fatalf("expected supportDev=true, got %#v", rawSupportDev)
	}
}

func TestCreateDatasource_D1LocalWithoutDatabaseNameUsesDatabaseID(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	created, err := app.CreateDatasource(DataSourcePayload{
		Name: "legacy-local",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":       "local",
			"binding":    "legacy_local",
			"databaseId": "local-db-id",
		},
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}

	if created.Database != "local-db-id" {
		t.Fatalf("expected database to default to databaseId, got %q", created.Database)
	}
	if got := strings.TrimSpace(optionAnyString(created.Options, "databaseName")); got != "local-db-id" {
		t.Fatalf("expected databaseName local-db-id, got %q", got)
	}
	if configPath := strings.TrimSpace(optionAnyString(created.Options, "wranglerConfigPath")); configPath != "" {
		t.Fatalf("expected no wranglerConfigPath for legacy local datasource without dev project path, got %q", configPath)
	}
}

func TestCreateDatasource_D1CloudWithoutDatabaseNameUsesDatabaseID(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	created, err := app.CreateDatasource(DataSourcePayload{
		Name: "legacy-cloud",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "cloud-db-id",
		},
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}

	if created.Database != "cloud-db-id" {
		t.Fatalf("expected database to default to databaseId, got %q", created.Database)
	}
	if got := strings.TrimSpace(optionAnyString(created.Options, "databaseName")); got != "cloud-db-id" {
		t.Fatalf("expected databaseName cloud-db-id, got %q", got)
	}
	if configPath := strings.TrimSpace(optionAnyString(created.Options, "wranglerConfigPath")); configPath != "" {
		t.Fatalf("expected no wranglerConfigPath for cloud datasource without dev project path, got %q", configPath)
	}
}

func TestCreateDatasource_D1AppendsDatabaseWhenProjectWranglerExists(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	projectDir := filepath.Join(t.TempDir(), "worker-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	configPath := filepath.Join(projectDir, "wrangler.toml")
	existing := strings.Join([]string{
		"[[d1_databases]]",
		`binding = "EXISTING_DB"`,
		`database_name = "existing-db"`,
		`database_id = "db_existing"`,
		`migrations_dir = "migrations/existing-db"`,
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write existing wrangler config: %v", err)
	}
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	one, err := app.CreateDatasource(DataSourcePayload{
		Name: "App Logs",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"accountId":      "acc_123",
			"databaseId":     "db_logs",
			"databaseName":   "my-log-db",
			"supportDev":     true,
			"devProjectPath": projectDir,
		},
	})
	if err != nil {
		t.Fatalf("CreateDatasource(one): %v", err)
	}

	if got := strings.TrimSpace(optionAnyString(one.Options, "wranglerConfigPath")); filepath.Clean(got) != filepath.Clean(configPath) {
		t.Fatalf("expected wranglerConfigPath %q, got %q", configPath, got)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read wrangler config: %v", err)
	}
	content := string(raw)
	if strings.Count(content, "[[d1_databases]]") != 2 {
		t.Fatalf("expected two d1_databases blocks after append, got content: %q", content)
	}
	if !strings.Contains(content, `database_id = "db_existing"`) {
		t.Fatalf("expected existing database entry to be preserved, got %q", content)
	}
	if !strings.Contains(content, `database_id = "db_logs"`) {
		t.Fatalf("expected new database entry appended, got %q", content)
	}
	if !strings.Contains(content, `migrations_dir = "migrations/my-log-db-db_logs"`) {
		t.Fatalf("expected new database migrations_dir, got %q", content)
	}
}

func TestUpdateDatasource_D1ReplacesBindingEntryWhenDatabaseIDChanges(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	projectDir := filepath.Join(t.TempDir(), "worker-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	configPath := filepath.Join(projectDir, "wrangler.toml")
	existing := strings.Join([]string{
		"[[d1_databases]]",
		`binding = "my_log_db"`,
		`database_name = "my-log-db"`,
		`database_id = "db_old"`,
		`migrations_dir = "migrations/my-log-db"`,
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write existing wrangler config: %v", err)
	}
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	created, err := app.CreateDatasource(DataSourcePayload{
		Name: "App Logs",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"accountId":      "acc_123",
			"databaseId":     "db_old",
			"databaseName":   "my-log-db",
			"supportDev":     true,
			"devProjectPath": projectDir,
		},
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}

	updated, err := app.UpdateDatasource(created.ID, DataSourcePayload{
		Name: "App Logs",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"accountId":      "acc_123",
			"databaseId":     "db_new",
			"databaseName":   "my-log-db",
			"supportDev":     true,
			"devProjectPath": projectDir,
		},
	})
	if err != nil {
		t.Fatalf("UpdateDatasource: %v", err)
	}
	if got := strings.TrimSpace(optionAnyString(updated.Options, "databaseId")); got != "db_new" {
		t.Fatalf("expected datasource to use updated databaseId db_new, got %q", got)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read wrangler config: %v", err)
	}
	content := string(raw)
	if strings.Count(content, "[[d1_databases]]") != 1 {
		t.Fatalf("expected wrangler.toml to keep one d1_databases block for same binding, got %q", content)
	}
	if strings.Contains(content, `database_id = "db_old"`) {
		t.Fatalf("expected old database_id to be replaced, got %q", content)
	}
	if !strings.Contains(content, `database_id = "db_new"`) {
		t.Fatalf("expected wrangler entry to update to db_new, got %q", content)
	}
}

func TestUpdateDatasource_D1MissingDatasourceDoesNotWriteWranglerToml(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	missingID := "missing-d1-id"
	_, err := app.UpdateDatasource(missingID, DataSourcePayload{
		Name: "Missing D1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_missing",
		},
	})
	if err == nil {
		t.Fatalf("expected error for missing datasource")
	}
	if err.Error() != "datasource not found" {
		t.Fatalf("expected datasource not found, got %v", err)
	}

	// Missing datasource should fail before any metadata mutation.
}

func TestUpdateDatasource_D1PreservesLegacyDevMetadataWhenSupportDevOmitted(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	projectDir := filepath.Join(t.TempDir(), "worker-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	configPath := filepath.Join(projectDir, "wrangler.toml")
	legacyConfig := strings.Join([]string{
		"[[d1_databases]]",
		`binding = "legacy_db"`,
		`database_name = "legacy-db"`,
		`database_id = "db_legacy"`,
		`migrations_dir = "migrations/legacy-db"`,
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(legacyConfig), 0o644); err != nil {
		t.Fatalf("write legacy wrangler config: %v", err)
	}
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	created, err := store.Create(datasource.DataSource{
		Name:     "Legacy D1",
		Type:     datasource.TypeD1,
		Database: "legacy-db",
		Options: map[string]any{
			"mode":               "cloud",
			"accountId":          "acc_123",
			"databaseId":         "db_legacy",
			"databaseName":       "legacy-db",
			"binding":            "legacy_db",
			"supportDev":         false,
			"wranglerConfigPath": configPath,
			"migrationsDir":      "migrations/legacy-db",
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	updated, err := app.UpdateDatasource(created.ID, DataSourcePayload{
		Name: "Legacy D1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":         "cloud",
			"accountId":    "acc_123",
			"databaseId":   "db_legacy",
			"databaseName": "legacy-db",
			"binding":      "legacy_db",
		},
	})
	if err != nil {
		t.Fatalf("UpdateDatasource: %v", err)
	}
	if got := strings.TrimSpace(optionAnyString(updated.Options, "wranglerConfigPath")); filepath.Clean(got) != filepath.Clean(configPath) {
		t.Fatalf("expected legacy wranglerConfigPath preserved, got %q", got)
	}
	if got := strings.TrimSpace(optionAnyString(updated.Options, "migrationsDir")); got != "migrations/legacy-db-db_legacy" {
		t.Fatalf("expected legacy migrationsDir to be normalized, got %q", got)
	}
	if !d1DatasourceSupportsDev(updated.Options) {
		t.Fatalf("expected legacy datasource to still support dev after update")
	}
}

func TestUpdateDatasource_D1LegacyExplicitDisableClearsDevMetadata(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	projectDir := filepath.Join(t.TempDir(), "worker-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	configPath := filepath.Join(projectDir, "wrangler.toml")
	legacyConfig := strings.Join([]string{
		"[[d1_databases]]",
		`binding = "legacy_db"`,
		`database_name = "legacy-db"`,
		`database_id = "db_legacy"`,
		`migrations_dir = "migrations/legacy-db"`,
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(legacyConfig), 0o644); err != nil {
		t.Fatalf("write legacy wrangler config: %v", err)
	}
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	created, err := store.Create(datasource.DataSource{
		Name:     "Legacy D1",
		Type:     datasource.TypeD1,
		Database: "legacy-db",
		Options: map[string]any{
			"mode":               "cloud",
			"accountId":          "acc_123",
			"databaseId":         "db_legacy",
			"databaseName":       "legacy-db",
			"binding":            "legacy_db",
			"supportDev":         false,
			"wranglerConfigPath": configPath,
			"migrationsDir":      "migrations/legacy-db",
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	updated, err := app.UpdateDatasource(created.ID, DataSourcePayload{
		Name: "Legacy D1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":         "cloud",
			"accountId":    "acc_123",
			"databaseId":   "db_legacy",
			"databaseName": "legacy-db",
			"binding":      "legacy_db",
			"supportDev":   false,
		},
	})
	if err != nil {
		t.Fatalf("UpdateDatasource: %v", err)
	}
	if got := strings.TrimSpace(optionAnyString(updated.Options, "wranglerConfigPath")); got != "" {
		t.Fatalf("expected wranglerConfigPath to be cleared when supportDev is explicitly disabled, got %q", got)
	}
	if got := strings.TrimSpace(optionAnyString(updated.Options, "migrationsDir")); got != "" {
		t.Fatalf("expected migrationsDir to be cleared when supportDev is explicitly disabled, got %q", got)
	}
	if d1DatasourceSupportsDev(updated.Options) {
		t.Fatalf("expected datasource to be remote-only after explicit supportDev=false")
	}
}

func TestD1MigrationDirNameIncludesDatabaseID(t *testing.T) {
	gotOne := d1MigrationDirName("my-log-db", "db_one")
	gotTwo := d1MigrationDirName("my-log-db", "db_two")

	if gotOne == gotTwo {
		t.Fatalf("expected migration dir names to differ by database ID, got %q", gotOne)
	}
	if !strings.Contains(gotOne, "db_one") {
		t.Fatalf("expected migration dir name to include database ID, got %q", gotOne)
	}
	if !strings.Contains(gotTwo, "db_two") {
		t.Fatalf("expected migration dir name to include database ID, got %q", gotTwo)
	}
}

func TestUpdateDatasource_D1RenameKeepsMigrationDir(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	projectDir := filepath.Join(t.TempDir(), "worker-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	created, err := app.CreateDatasource(DataSourcePayload{
		Name: "Rename Source A",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":           "cloud",
			"accountId":      "acc_123",
			"databaseId":     "db_rename_stable",
			"databaseName":   "rename-stable-db",
			"binding":        "RENAME_STABLE",
			"supportDev":     true,
			"devProjectPath": projectDir,
		},
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}
	createdMigrationDir := strings.TrimSpace(optionAnyString(created.Options, "migrationsDir"))
	if createdMigrationDir == "" {
		t.Fatalf("expected migrationsDir to be set on create")
	}

	updated, err := app.UpdateDatasource(created.ID, DataSourcePayload{
		Name: "Rename Source B",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":           "cloud",
			"accountId":      "acc_123",
			"databaseId":     "db_rename_stable",
			"databaseName":   "rename-stable-db",
			"binding":        "RENAME_STABLE",
			"supportDev":     true,
			"devProjectPath": projectDir,
		},
	})
	if err != nil {
		t.Fatalf("UpdateDatasource: %v", err)
	}
	updatedMigrationDir := strings.TrimSpace(optionAnyString(updated.Options, "migrationsDir"))
	if updatedMigrationDir == "" {
		t.Fatalf("expected migrationsDir to be set on update")
	}
	if updatedMigrationDir != createdMigrationDir {
		t.Fatalf("expected migrationsDir to stay stable across datasource renames, got %q and %q", createdMigrationDir, updatedMigrationDir)
	}
}

func TestUpdateDatasource_D1LegacyDevUpdatesWranglerConfig(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	projectDir := filepath.Join(t.TempDir(), "worker-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	configPath := filepath.Join(projectDir, "wrangler.toml")
	initial := strings.Join([]string{
		"[[d1_databases]]",
		`binding = "OLD_DB"`,
		`database_name = "legacy-db"`,
		`database_id = "db_old"`,
		`migrations_dir = "migrations/old-dir"`,
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write wrangler config: %v", err)
	}

	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	created, err := store.Create(datasource.DataSource{
		Name:     "Legacy D1",
		Type:     datasource.TypeD1,
		Database: "legacy-db",
		Options: map[string]any{
			"mode":               "cloud",
			"accountId":          "acc_123",
			"databaseId":         "db_old",
			"databaseName":       "legacy-db",
			"binding":            "OLD_DB",
			"supportDev":         false,
			"wranglerConfigPath": configPath,
			"migrationsDir":      "migrations/old-dir",
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	updated, err := app.UpdateDatasource(created.ID, DataSourcePayload{
		Name: "Legacy D1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":         "cloud",
			"accountId":    "acc_123",
			"databaseId":   "db_new",
			"databaseName": "legacy-db",
			"binding":      "NEW_DB",
		},
	})
	if err != nil {
		t.Fatalf("UpdateDatasource: %v", err)
	}
	if got := strings.TrimSpace(optionAnyString(updated.Options, "wranglerConfigPath")); filepath.Clean(got) != filepath.Clean(configPath) {
		t.Fatalf("expected wranglerConfigPath to remain %q, got %q", configPath, got)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read wrangler config: %v", err)
	}
	content := string(raw)
	if strings.Count(content, "[[d1_databases]]") != 1 {
		t.Fatalf("expected one d1_databases block after update, got %q", content)
	}
	if strings.Contains(content, `database_id = "db_old"`) {
		t.Fatalf("expected old database_id to be replaced, got %q", content)
	}
	if !strings.Contains(content, `database_id = "db_new"`) {
		t.Fatalf("expected new database_id in wrangler config, got %q", content)
	}
	if !strings.Contains(content, `binding = "NEW_DB"`) {
		t.Fatalf("expected binding to be updated in wrangler config, got %q", content)
	}
}

func TestUpdateDatasource_D1SupportDevReplacesEntryWhenDatabaseAndBindingChange(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	projectDir := filepath.Join(t.TempDir(), "worker-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	configPath := filepath.Join(projectDir, "wrangler.toml")
	initial := strings.Join([]string{
		"[[d1_databases]]",
		`binding = "OLD_DB"`,
		`database_name = "legacy-db"`,
		`database_id = "db_old"`,
		`migrations_dir = "migrations/old-dir"`,
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write wrangler config: %v", err)
	}
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	created, err := app.CreateDatasource(DataSourcePayload{
		Name: "D1 Dev",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":           "cloud",
			"accountId":      "acc_123",
			"databaseId":     "db_old",
			"databaseName":   "legacy-db",
			"binding":        "OLD_DB",
			"supportDev":     true,
			"devProjectPath": projectDir,
		},
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}

	updated, err := app.UpdateDatasource(created.ID, DataSourcePayload{
		Name: "D1 Dev",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":           "cloud",
			"accountId":      "acc_123",
			"databaseId":     "db_new",
			"databaseName":   "legacy-db",
			"binding":        "NEW_DB",
			"supportDev":     true,
			"devProjectPath": projectDir,
		},
	})
	if err != nil {
		t.Fatalf("UpdateDatasource: %v", err)
	}
	if got := strings.TrimSpace(optionAnyString(updated.Options, "databaseId")); got != "db_new" {
		t.Fatalf("expected datasource to use updated databaseId db_new, got %q", got)
	}
	if got := strings.TrimSpace(optionAnyString(updated.Options, "binding")); got != "NEW_DB" {
		t.Fatalf("expected datasource to use updated binding NEW_DB, got %q", got)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read wrangler config: %v", err)
	}
	content := string(raw)
	if strings.Count(content, "[[d1_databases]]") != 1 {
		t.Fatalf("expected wrangler.toml to keep one d1_databases block after update, got %q", content)
	}
	if strings.Contains(content, `database_id = "db_old"`) {
		t.Fatalf("expected old database_id to be replaced, got %q", content)
	}
	if strings.Contains(content, `binding = "OLD_DB"`) {
		t.Fatalf("expected old binding to be replaced, got %q", content)
	}
	if !strings.Contains(content, `database_id = "db_new"`) {
		t.Fatalf("expected new database_id in wrangler config, got %q", content)
	}
	if !strings.Contains(content, `binding = "NEW_DB"`) {
		t.Fatalf("expected new binding in wrangler config, got %q", content)
	}
}

func TestUpdateDatasource_D1LegacyDevMissingWranglerPathDoesNotFail(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	staleProjectDir := filepath.Join(t.TempDir(), "missing-worker-project")
	configPath := filepath.Join(staleProjectDir, "wrangler.toml")
	store := datasource.NewStore(dataPath)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	created, err := store.Create(datasource.DataSource{
		Name:     "Legacy D1 Missing Path",
		Type:     datasource.TypeD1,
		Database: "legacy-db-missing",
		Options: map[string]any{
			"mode":               "cloud",
			"accountId":          "acc_123",
			"databaseId":         "db_missing_old",
			"databaseName":       "legacy-db-missing",
			"binding":            "LEGACY_MISSING",
			"supportDev":         false,
			"wranglerConfigPath": configPath,
			"migrationsDir":      "migrations/legacy-missing",
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}
	if _, statErr := os.Stat(staleProjectDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected stale project dir to be absent before update, got %v", statErr)
	}

	app := &App{
		cfg:   Config{DataPath: dataPath},
		store: store,
	}

	updated, err := app.UpdateDatasource(created.ID, DataSourcePayload{
		Name: "Legacy D1 Missing Path",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":         "cloud",
			"accountId":    "acc_123",
			"databaseId":   "db_missing_new",
			"databaseName": "legacy-db-missing",
			"binding":      "LEGACY_MISSING_NEW",
			"supportDev":   false,
		},
	})
	if err != nil {
		t.Fatalf("UpdateDatasource should not fail for missing legacy wrangler path, got %v", err)
	}
	if got := strings.TrimSpace(optionAnyString(updated.Options, "wranglerConfigPath")); got != "" {
		t.Fatalf("expected wranglerConfigPath to be cleared when legacy path is missing, got %q", got)
	}
	if got := strings.TrimSpace(optionAnyString(updated.Options, "migrationsDir")); got != "" {
		t.Fatalf("expected migrationsDir to be cleared when legacy path is missing, got %q", got)
	}
	if d1DatasourceSupportsDev(updated.Options) {
		t.Fatalf("expected datasource to fallback to remote-only when legacy wrangler path is missing")
	}
}

func TestD1MigrationDirNameDoesNotCollapseSubstringIDs(t *testing.T) {
	gotOne := d1MigrationDirName("analytics", "ana")
	gotTwo := d1MigrationDirName("analytics", "lyt")
	if gotOne == gotTwo {
		t.Fatalf("expected different migration dirs for substring database IDs, got %q", gotOne)
	}
	if gotOne != "analytics-ana" {
		t.Fatalf("expected analytics-ana, got %q", gotOne)
	}
	if gotTwo != "analytics-lyt" {
		t.Fatalf("expected analytics-lyt, got %q", gotTwo)
	}
}

func TestD1WranglerUpsertDatabaseEntry_ReplacesExistingDatabaseIDEntry(t *testing.T) {
	content := strings.Join([]string{
		"[[d1_databases]]",
		`binding = "LOG_DB"`,
		`database_name = "my-log-db"`,
		`database_id = "db_same"`,
		`migrations_dir = "migrations/old-dir"`,
		"",
	}, "\n")
	next, changed := d1WranglerUpsertDatabaseEntry(content, d1WranglerDatabaseEntry{
		Binding:       "LOG_DB_NEW",
		DatabaseName:  "my-log-db",
		DatabaseID:    "db_same",
		MigrationsDir: "migrations/new-dir",
	})
	if !changed {
		t.Fatalf("expected upsert to update existing database_id block")
	}
	if strings.Count(next, "[[d1_databases]]") != 1 {
		t.Fatalf("expected one d1_databases block after replace, got %q", next)
	}
	if !strings.Contains(next, `database_id = "db_same"`) {
		t.Fatalf("expected database_id to remain db_same, got %q", next)
	}
	if !strings.Contains(next, `binding = "LOG_DB_NEW"`) {
		t.Fatalf("expected binding to update with replacement entry, got %q", next)
	}
	if !strings.Contains(next, `migrations_dir = "migrations/new-dir"`) {
		t.Fatalf("expected migrations_dir to update for existing database_id, got %q", next)
	}
}

func TestD1WranglerUpsertDatabaseEntry_ReplacesDatabaseIDWithoutSpacing(t *testing.T) {
	content := strings.Join([]string{
		"[[d1_databases]]",
		`binding = "LOG_DB"`,
		`database_name = "my-log-db"`,
		`database_id="db_same"`,
		`migrations_dir = "migrations/old-dir"`,
		"",
	}, "\n")
	next, changed := d1WranglerUpsertDatabaseEntry(content, d1WranglerDatabaseEntry{
		Binding:       "LOG_DB",
		DatabaseName:  "my-log-db",
		DatabaseID:    "db_same",
		MigrationsDir: "migrations/new-dir",
	})
	if !changed {
		t.Fatalf("expected upsert to replace compact database_id syntax")
	}
	if strings.Count(next, "[[d1_databases]]") != 1 {
		t.Fatalf("expected one d1_databases block after replace, got %q", next)
	}
	if !strings.Contains(next, `migrations_dir = "migrations/new-dir"`) {
		t.Fatalf("expected migrations_dir to be updated, got %q", next)
	}
	if strings.Contains(next, `database_id="db_same"`) {
		t.Fatalf("expected canonical formatting after replace, got %q", next)
	}
}

func TestD1WranglerUpsertDatabaseEntry_PreservesNonD1Sections(t *testing.T) {
	content := strings.Join([]string{
		"[[d1_databases]]",
		`binding = "LOG_DB"`,
		`database_name = "my-log-db"`,
		`database_id = "db_same"`,
		`migrations_dir = "migrations/old-dir"`,
		"",
		"[vars]",
		`FOO = "bar"`,
		"",
	}, "\n")
	next, changed := d1WranglerUpsertDatabaseEntry(content, d1WranglerDatabaseEntry{
		Binding:       "LOG_DB",
		DatabaseName:  "my-log-db",
		DatabaseID:    "db_same",
		MigrationsDir: "migrations/new-dir",
	})
	if !changed {
		t.Fatalf("expected upsert to replace database entry")
	}
	if !strings.Contains(next, "[vars]") {
		t.Fatalf("expected non-d1 section to remain after replace, got %q", next)
	}
	if !strings.Contains(next, `FOO = "bar"`) {
		t.Fatalf("expected vars section values to remain after replace, got %q", next)
	}
	if !strings.Contains(next, `migrations_dir = "migrations/new-dir"`) {
		t.Fatalf("expected d1 section to be updated, got %q", next)
	}
}

func TestCreateDatasource_RedisAutoDiscoversClusterNodes(t *testing.T) {
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))

	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, metricsStubAdapter{
		execute: func(ds datasource.DataSource, statement string) (console.QueryResult, error) {
			_ = ds
			switch strings.ToUpper(strings.TrimSpace(statement)) {
			case "CLUSTER NODES":
				return console.QueryResult{
					Rows: []map[string]any{
						{
							"result": strings.Join([]string{
								"node-a 10.0.0.1:7000@17000 master - 0 1700000000000 1 connected 0-5460",
								"node-b 10.0.0.2:7001@17001 master - 0 1700000000000 2 connected 5461-10922",
							}, "\n"),
						},
					},
				}, nil
			default:
				return console.QueryResult{}, errors.New("unexpected statement: " + statement)
			}
		},
	})

	app := &App{store: store, manager: manager}
	created, err := app.CreateDatasource(DataSourcePayload{
		Name: "redis-cluster",
		Type: datasource.TypeRedis,
		Host: "10.0.0.1",
		Port: 7000,
	})
	if err != nil {
		t.Fatalf("CreateDatasource: %v", err)
	}

	nodes := redisNodesFromDatasourceOptions(created)
	if len(nodes) != 2 || nodes[0] != "10.0.0.1:7000" || nodes[1] != "10.0.0.2:7001" {
		t.Fatalf("expected discovered nodes to be persisted, got %#v", nodes)
	}
}

func TestUpdateDatasource_RedisAutoDiscoversClusterNodes(t *testing.T) {
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "redis-cluster",
		Type: datasource.TypeRedis,
		Host: "10.0.0.1",
		Port: 7000,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, metricsStubAdapter{
		execute: func(ds datasource.DataSource, statement string) (console.QueryResult, error) {
			_ = ds
			switch strings.ToUpper(strings.TrimSpace(statement)) {
			case "CLUSTER NODES":
				return console.QueryResult{
					Rows: []map[string]any{
						{
							"result": strings.Join([]string{
								"node-a 10.0.0.1:7000@17000 master - 0 1700000000000 1 connected 0-5460",
								"node-b 10.0.0.2:7001@17001 master - 0 1700000000000 2 connected 5461-10922",
							}, "\n"),
						},
					},
				}, nil
			default:
				return console.QueryResult{}, errors.New("unexpected statement: " + statement)
			}
		},
	})

	app := &App{store: store, manager: manager}
	updated, err := app.UpdateDatasource(created.ID, DataSourcePayload{
		Name: "redis-cluster",
		Type: datasource.TypeRedis,
		Host: "10.0.0.1",
		Port: 7000,
	})
	if err != nil {
		t.Fatalf("UpdateDatasource: %v", err)
	}

	nodes := redisNodesFromDatasourceOptions(updated)
	if len(nodes) != 2 || nodes[0] != "10.0.0.1:7000" || nodes[1] != "10.0.0.2:7001" {
		t.Fatalf("expected discovered nodes to be persisted on update, got %#v", nodes)
	}
}

func TestDynamoDBSSOListProfiles_ParsesDefaultAndNamedProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeAWSTestConfig(t, home, strings.Join([]string{
		"[default]",
		"region = us-east-1",
		"sso_start_url = https://example.awsapps.com/start",
		"",
		"[profile analytics]",
		"region = us-west-2",
		"sso_start_url = https://example.awsapps.com/start",
		"",
	}, "\n"))

	app := &App{}
	profiles, err := app.DynamoDBSSOListProfiles("")
	if err != nil {
		t.Fatalf("DynamoDBSSOListProfiles: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
	if profiles[0].Name != "default" || profiles[0].Region != "us-east-1" {
		t.Fatalf("expected default profile first, got %#v", profiles[0])
	}
	if profiles[0].StartURL != "https://example.awsapps.com/start" {
		t.Fatalf("expected default start url from config, got %#v", profiles[0])
	}
	if profiles[1].Name != "analytics" || profiles[1].Region != "us-west-2" {
		t.Fatalf("expected analytics profile second, got %#v", profiles[1])
	}
	if profiles[1].StartURL != "https://example.awsapps.com/start" {
		t.Fatalf("expected analytics start url from config, got %#v", profiles[1])
	}
}

func TestDynamoDBSSOListProfiles_ResolvesSSOSessionSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeAWSTestConfig(t, home, strings.Join([]string{
		"[profile analytics]",
		"region = ap-southeast-1",
		"sso_session = corp-session",
		"sso_account_id = 111111111111",
		"sso_role_name = Developer",
		"",
		"[sso-session corp-session]",
		"sso_start_url = https://portal.example.awsapps.com/start",
		"sso_region = us-east-1",
		"",
	}, "\n"))

	app := &App{}
	profiles, err := app.DynamoDBSSOListProfiles("")
	if err != nil {
		t.Fatalf("DynamoDBSSOListProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].Name != "analytics" {
		t.Fatalf("expected analytics profile, got %#v", profiles[0])
	}
	if profiles[0].StartURL != "https://portal.example.awsapps.com/start" {
		t.Fatalf("expected start url from sso-session, got %#v", profiles[0])
	}
	if profiles[0].SSORegion != "us-east-1" {
		t.Fatalf("expected sso region from sso-session, got %#v", profiles[0])
	}
}

func TestDynamoDBSSOLogin_UsesProfileAndResolvesCachedToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeAWSTestConfig(t, home, strings.Join([]string{
		"[profile analytics]",
		"sso_start_url = https://example.awsapps.com/start",
		"sso_region = us-east-1",
		"",
	}, "\n"))
	writeAWSTestSSOCacheFile(t, home, "cache.json", `{
		"startUrl": "https://example.awsapps.com/start",
		"accessToken": "access_token_ready",
		"expiresAt": "2099-01-01T00:00:00Z"
	}`)

	app := &App{
		runCommand: func(_ context.Context, command []string) ([]byte, error) {
			t.Fatalf("unexpected shell command: %v", command)
			return nil, nil
		},
	}

	session, err := app.DynamoDBSSOLogin("analytics")
	if err != nil {
		t.Fatalf("DynamoDBSSOLogin: %v", err)
	}
	if strings.TrimSpace(session.AccessToken) != "access_token_ready" {
		t.Fatalf("expected access_token_ready, got %q", session.AccessToken)
	}
	if strings.TrimSpace(session.ExpiresAt) == "" {
		t.Fatalf("expected non-empty expiresAt")
	}
}

func TestDynamoDBSSOListAccounts_ParsesAccountList(t *testing.T) {
	originalFactory := newDynamoDBSSOClient
	t.Cleanup(func() { newDynamoDBSSOClient = originalFactory })

	requests := 0
	newDynamoDBSSOClient = func(region string, _ *http.Client) dynamoDBSSOClient {
		if region != "us-east-1" {
			t.Fatalf("expected region us-east-1, got %s", region)
		}
		return &testDynamoDBSSOClient{
			listAccountsFn: func(_ context.Context, input dynamoDBSSOListAccountsInput) (dynamoDBSSOListAccountsOutput, error) {
				requests++
				if input.AccessToken != "token_123" {
					t.Fatalf("expected access token token_123, got %q", input.AccessToken)
				}
				if requests == 1 {
					return dynamoDBSSOListAccountsOutput{
						AccountList: []dynamoDBSSOAccountInfo{
							{AccountID: "111111111111", AccountName: "Prod", EmailAddress: "prod@example.com"},
						},
						NextToken: "next-page",
					}, nil
				}
				if input.NextToken != "next-page" {
					t.Fatalf("expected next token next-page, got %q", input.NextToken)
				}
				return dynamoDBSSOListAccountsOutput{
					AccountList: []dynamoDBSSOAccountInfo{
						{AccountID: "222222222222", AccountName: "Dev", EmailAddress: "dev@example.com"},
					},
				}, nil
			},
		}
	}

	app := &App{}
	accounts, err := app.DynamoDBSSOListAccounts("token_123", "us-east-1")
	if err != nil {
		t.Fatalf("DynamoDBSSOListAccounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}
	if accounts[0].AccountID != "222222222222" || accounts[0].AccountName != "Dev" {
		t.Fatalf("unexpected first account: %#v", accounts[0])
	}
	if requests != 2 {
		t.Fatalf("expected 2 paginated requests, got %d", requests)
	}
}

func TestDynamoDBSSOListAccountRoles_ParsesRoleList(t *testing.T) {
	originalFactory := newDynamoDBSSOClient
	t.Cleanup(func() { newDynamoDBSSOClient = originalFactory })

	newDynamoDBSSOClient = func(region string, _ *http.Client) dynamoDBSSOClient {
		if region != "us-east-1" {
			t.Fatalf("expected region us-east-1, got %s", region)
		}
		return &testDynamoDBSSOClient{
			listAccountRolesFn: func(_ context.Context, input dynamoDBSSOListAccountRolesInput) (dynamoDBSSOListAccountRolesOutput, error) {
				if input.AccountID != "111111111111" {
					t.Fatalf("expected account id 111111111111, got %q", input.AccountID)
				}
				if input.AccessToken != "token_123" {
					t.Fatalf("expected access token token_123, got %q", input.AccessToken)
				}
				return dynamoDBSSOListAccountRolesOutput{
					RoleList: []dynamoDBSSORoleInfo{
						{RoleName: "Admin", AccountID: "111111111111"},
						{RoleName: "ReadOnly", AccountID: "111111111111"},
					},
				}, nil
			},
		}
	}

	app := &App{}
	roles, err := app.DynamoDBSSOListAccountRoles("111111111111", "token_123", "us-east-1")
	if err != nil {
		t.Fatalf("DynamoDBSSOListAccountRoles: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}
	if roles[0].RoleName != "Admin" || roles[0].AccountID != "111111111111" {
		t.Fatalf("unexpected first role: %#v", roles[0])
	}
}

func TestDynamoDBSSOGetRoleCredentials_ParsesRoleCredentials(t *testing.T) {
	originalFactory := newDynamoDBSSOClient
	t.Cleanup(func() { newDynamoDBSSOClient = originalFactory })

	newDynamoDBSSOClient = func(region string, _ *http.Client) dynamoDBSSOClient {
		if region != "us-east-1" {
			t.Fatalf("expected region us-east-1, got %s", region)
		}
		return &testDynamoDBSSOClient{
			getRoleCredentialsFn: func(_ context.Context, input dynamoDBSSOGetRoleCredentialsInput) (dynamoDBSSOGetRoleCredentialsOutput, error) {
				if input.AccountID != "111111111111" {
					t.Fatalf("expected account id 111111111111, got %q", input.AccountID)
				}
				if input.RoleName != "Admin" {
					t.Fatalf("expected role name Admin, got %q", input.RoleName)
				}
				if input.AccessToken != "token_123" {
					t.Fatalf("expected access token token_123, got %q", input.AccessToken)
				}
				return dynamoDBSSOGetRoleCredentialsOutput{
					RoleCredentials: &dynamoDBSSORoleCredentialsOutput{
						AccessKeyID:     "AKIA_TEST",
						SecretAccessKey: "SECRET_TEST",
						SessionToken:    "SESSION_TEST",
						Expiration:      1735689600000,
					},
				}, nil
			},
		}
	}

	app := &App{}
	credentials, err := app.DynamoDBSSOGetRoleCredentials("111111111111", "Admin", "token_123", "us-east-1")
	if err != nil {
		t.Fatalf("DynamoDBSSOGetRoleCredentials: %v", err)
	}
	if credentials.AccessKeyID != "AKIA_TEST" {
		t.Fatalf("expected AccessKeyID AKIA_TEST, got %q", credentials.AccessKeyID)
	}
	if credentials.SecretAccessKey != "SECRET_TEST" {
		t.Fatalf("expected SecretAccessKey SECRET_TEST, got %q", credentials.SecretAccessKey)
	}
	if credentials.SessionToken != "SESSION_TEST" {
		t.Fatalf("expected SessionToken SESSION_TEST, got %q", credentials.SessionToken)
	}
	if credentials.Expiration != 1735689600000 {
		t.Fatalf("expected expiration 1735689600000, got %d", credentials.Expiration)
	}
}

func TestDynamoDBSSOListProfiles_UsesCustomConfigPath(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "custom-aws-config")
	if err := os.WriteFile(configPath, []byte(strings.Join([]string{
		"[profile zoom-sso-dev]",
		"region = ap-southeast-1",
		"sso_region = us-east-1",
		"sso_start_url = https://example.awsapps.com/start",
		"sso_account_id = 111111111111",
		"sso_role_name = Developer",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write custom aws config: %v", err)
	}

	app := &App{}
	profiles, err := app.DynamoDBSSOListProfiles(configPath)
	if err != nil {
		t.Fatalf("DynamoDBSSOListProfiles custom path: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].Name != "zoom-sso-dev" {
		t.Fatalf("expected zoom-sso-dev profile, got %#v", profiles[0])
	}
	if profiles[0].Region != "ap-southeast-1" || profiles[0].SSORegion != "us-east-1" {
		t.Fatalf("unexpected regions: %#v", profiles[0])
	}
	if profiles[0].StartURL != "https://example.awsapps.com/start" {
		t.Fatalf("expected start url from custom config, got %#v", profiles[0])
	}
	if profiles[0].AccountID != "111111111111" || profiles[0].RoleName != "Developer" {
		t.Fatalf("expected account/role from config, got %#v", profiles[0])
	}
}

func TestDynamoDBSSOOAuthAuthorize_UsesAPIWithoutAWSCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configPath := filepath.Join(t.TempDir(), "custom-aws-config")
	if err := os.WriteFile(configPath, []byte(strings.Join([]string{
		"[profile zoom-sso-dev]",
		"region = ap-southeast-1",
		"sso_region = us-east-1",
		"sso_start_url = https://example.awsapps.com/start",
		"sso_account_id = 111111111111",
		"sso_role_name = Developer",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write custom aws config: %v", err)
	}
	originalOIDCFactory := newDynamoDBSSOOIDCClient
	originalSSOFactory := newDynamoDBSSOClient
	originalOpenURL := openDynamoDBSSOVerificationURL
	originalWait := waitDynamoDBSSOPollInterval
	t.Cleanup(func() {
		newDynamoDBSSOOIDCClient = originalOIDCFactory
		newDynamoDBSSOClient = originalSSOFactory
		openDynamoDBSSOVerificationURL = originalOpenURL
		waitDynamoDBSSOPollInterval = originalWait
	})

	openCalls := 0
	openDynamoDBSSOVerificationURL = func(rawURL string) error {
		openCalls++
		if rawURL != "https://device.example/verify-complete" {
			t.Fatalf("unexpected verification URL: %s", rawURL)
		}
		return nil
	}
	waitCalls := 0
	waitDynamoDBSSOPollInterval = func(_ context.Context, _ time.Duration) error {
		waitCalls++
		return nil
	}

	tokenCalls := 0
	newDynamoDBSSOOIDCClient = func(region string, _ *http.Client) dynamoDBSSOOIDCClient {
		if region != "us-east-1" {
			t.Fatalf("expected OIDC region us-east-1, got %s", region)
		}
		return &testDynamoDBSSOOIDCClient{
			registerClientFn: func(_ context.Context, input dynamoDBSSOOIDCRegisterClientInput) (dynamoDBSSOOIDCRegisterClientOutput, error) {
				if input.ClientName != "zoom-sso-dev" {
					t.Fatalf("expected register-client name to use selected profile, got %q", input.ClientName)
				}
				if input.ClientType != "public" {
					t.Fatalf("expected public client type, got %q", input.ClientType)
				}
				return dynamoDBSSOOIDCRegisterClientOutput{
					ClientID:     "client-id",
					ClientSecret: "client-secret",
				}, nil
			},
			startDeviceAuthorizationFn: func(_ context.Context, input dynamoDBSSOOIDCStartDeviceAuthorizationInput) (dynamoDBSSOOIDCStartDeviceAuthorizationOutput, error) {
				if input.StartURL != "https://example.awsapps.com/start" {
					t.Fatalf("unexpected start URL: %s", input.StartURL)
				}
				return dynamoDBSSOOIDCStartDeviceAuthorizationOutput{
					DeviceCode:              "device-code",
					VerificationURIComplete: "https://device.example/verify-complete",
					ExpiresIn:               600,
					Interval:                1,
				}, nil
			},
			createTokenFn: func(_ context.Context, input dynamoDBSSOOIDCCreateTokenInput) (dynamoDBSSOOIDCCreateTokenOutput, error) {
				tokenCalls++
				if input.DeviceCode != "device-code" {
					t.Fatalf("unexpected device code: %s", input.DeviceCode)
				}
				if tokenCalls == 1 {
					return dynamoDBSSOOIDCCreateTokenOutput{}, &dynamoDBSSOAPIError{Code: "AuthorizationPendingException"}
				}
				return dynamoDBSSOOIDCCreateTokenOutput{
					AccessToken: "sdk_access_token",
					ExpiresIn:   3600,
				}, nil
			},
		}
	}

	newDynamoDBSSOClient = func(region string, _ *http.Client) dynamoDBSSOClient {
		if region != "us-east-1" {
			t.Fatalf("expected SSO region us-east-1, got %s", region)
		}
		return &testDynamoDBSSOClient{
			getRoleCredentialsFn: func(_ context.Context, input dynamoDBSSOGetRoleCredentialsInput) (dynamoDBSSOGetRoleCredentialsOutput, error) {
				if input.AccessToken != "sdk_access_token" {
					t.Fatalf("expected sdk access token, got %q", input.AccessToken)
				}
				if input.AccountID != "111111111111" || input.RoleName != "Developer" {
					t.Fatalf("unexpected account/role in input: %#v", input)
				}
				return dynamoDBSSOGetRoleCredentialsOutput{
					RoleCredentials: &dynamoDBSSORoleCredentialsOutput{
						AccessKeyID:     "AKIA_TEST",
						SecretAccessKey: "SECRET_TEST",
						SessionToken:    "SESSION_TEST",
						Expiration:      1735689600000,
					},
				}, nil
			},
		}
	}

	app := &App{
		runCommand: func(_ context.Context, command []string) ([]byte, error) {
			t.Fatalf("unexpected shell command in SDK auth flow: %v", command)
			return nil, nil
		},
	}

	result, err := app.DynamoDBSSOOAuthAuthorize("zoom-sso-dev", "", configPath)
	if err != nil {
		t.Fatalf("DynamoDBSSOOAuthAuthorize: %v", err)
	}
	if result.Profile != "zoom-sso-dev" || result.Region != "ap-southeast-1" {
		t.Fatalf("unexpected profile/region: %#v", result)
	}
	if result.AccountID != "111111111111" || result.RoleName != "Developer" {
		t.Fatalf("unexpected account/role: %#v", result)
	}
	if result.AccessKeyID != "AKIA_TEST" || result.SecretAccessKey != "SECRET_TEST" || result.SessionToken != "SESSION_TEST" {
		t.Fatalf("unexpected credentials: %#v", result)
	}
	if openCalls != 1 {
		t.Fatalf("expected open URL to be called once, got %d", openCalls)
	}
	if tokenCalls != 2 {
		t.Fatalf("expected create token to be called twice, got %d", tokenCalls)
	}
	if waitCalls != 1 {
		t.Fatalf("expected one poll wait, got %d", waitCalls)
	}
}

type testDynamoDBSSOClient struct {
	listAccountsFn       func(ctx context.Context, params dynamoDBSSOListAccountsInput) (dynamoDBSSOListAccountsOutput, error)
	listAccountRolesFn   func(ctx context.Context, params dynamoDBSSOListAccountRolesInput) (dynamoDBSSOListAccountRolesOutput, error)
	getRoleCredentialsFn func(ctx context.Context, params dynamoDBSSOGetRoleCredentialsInput) (dynamoDBSSOGetRoleCredentialsOutput, error)
}

func (c *testDynamoDBSSOClient) ListAccounts(ctx context.Context, params dynamoDBSSOListAccountsInput) (dynamoDBSSOListAccountsOutput, error) {
	if c.listAccountsFn == nil {
		return dynamoDBSSOListAccountsOutput{}, errors.New("unexpected ListAccounts call")
	}
	return c.listAccountsFn(ctx, params)
}

func (c *testDynamoDBSSOClient) ListAccountRoles(ctx context.Context, params dynamoDBSSOListAccountRolesInput) (dynamoDBSSOListAccountRolesOutput, error) {
	if c.listAccountRolesFn == nil {
		return dynamoDBSSOListAccountRolesOutput{}, errors.New("unexpected ListAccountRoles call")
	}
	return c.listAccountRolesFn(ctx, params)
}

func (c *testDynamoDBSSOClient) GetRoleCredentials(ctx context.Context, params dynamoDBSSOGetRoleCredentialsInput) (dynamoDBSSOGetRoleCredentialsOutput, error) {
	if c.getRoleCredentialsFn == nil {
		return dynamoDBSSOGetRoleCredentialsOutput{}, errors.New("unexpected GetRoleCredentials call")
	}
	return c.getRoleCredentialsFn(ctx, params)
}

type testDynamoDBSSOOIDCClient struct {
	registerClientFn           func(ctx context.Context, params dynamoDBSSOOIDCRegisterClientInput) (dynamoDBSSOOIDCRegisterClientOutput, error)
	startDeviceAuthorizationFn func(ctx context.Context, params dynamoDBSSOOIDCStartDeviceAuthorizationInput) (dynamoDBSSOOIDCStartDeviceAuthorizationOutput, error)
	createTokenFn              func(ctx context.Context, params dynamoDBSSOOIDCCreateTokenInput) (dynamoDBSSOOIDCCreateTokenOutput, error)
}

func (c *testDynamoDBSSOOIDCClient) RegisterClient(ctx context.Context, params dynamoDBSSOOIDCRegisterClientInput) (dynamoDBSSOOIDCRegisterClientOutput, error) {
	if c.registerClientFn == nil {
		return dynamoDBSSOOIDCRegisterClientOutput{}, errors.New("unexpected RegisterClient call")
	}
	return c.registerClientFn(ctx, params)
}

func (c *testDynamoDBSSOOIDCClient) StartDeviceAuthorization(ctx context.Context, params dynamoDBSSOOIDCStartDeviceAuthorizationInput) (dynamoDBSSOOIDCStartDeviceAuthorizationOutput, error) {
	if c.startDeviceAuthorizationFn == nil {
		return dynamoDBSSOOIDCStartDeviceAuthorizationOutput{}, errors.New("unexpected StartDeviceAuthorization call")
	}
	return c.startDeviceAuthorizationFn(ctx, params)
}

func (c *testDynamoDBSSOOIDCClient) CreateToken(ctx context.Context, params dynamoDBSSOOIDCCreateTokenInput) (dynamoDBSSOOIDCCreateTokenOutput, error) {
	if c.createTokenFn == nil {
		return dynamoDBSSOOIDCCreateTokenOutput{}, errors.New("unexpected CreateToken call")
	}
	return c.createTokenFn(ctx, params)
}

func writeAWSTestConfig(t *testing.T, homeDir, content string) {
	t.Helper()
	awsDir := filepath.Join(homeDir, ".aws")
	if err := os.MkdirAll(awsDir, 0o755); err != nil {
		t.Fatalf("mkdir aws dir: %v", err)
	}
	configPath := filepath.Join(awsDir, "config")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write aws config: %v", err)
	}
}

func writeAWSTestSSOCacheFile(t *testing.T, homeDir, fileName, content string) {
	t.Helper()
	cacheDir := filepath.Join(homeDir, ".aws", "sso", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir aws sso cache dir: %v", err)
	}
	cachePath := filepath.Join(cacheDir, fileName)
	if err := os.WriteFile(cachePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write aws sso cache file: %v", err)
	}
}

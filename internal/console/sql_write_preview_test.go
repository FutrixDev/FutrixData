package console

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"futrixdata/platform/internal/datasource"
)

const stubMySQLWritePreviewDriverName = "stub-mysql-write-preview"

var (
	registerStubMySQLWritePreviewDriverOnce sync.Once
	stubMySQLWritePreviewQueries            []string
)

func registerStubMySQLWritePreviewDriver() {
	registerStubMySQLWritePreviewDriverOnce.Do(func() {
		sql.Register(stubMySQLWritePreviewDriverName, stubMySQLWritePreviewDriver{})
	})
}

type stubMySQLWritePreviewDriver struct{}

func (stubMySQLWritePreviewDriver) Open(name string) (driver.Conn, error) {
	return &stubMySQLWritePreviewConn{}, nil
}

type stubMySQLWritePreviewConn struct{}

var _ driver.Conn = (*stubMySQLWritePreviewConn)(nil)
var _ driver.QueryerContext = (*stubMySQLWritePreviewConn)(nil)

func (c *stubMySQLWritePreviewConn) Prepare(query string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported")
}

func (c *stubMySQLWritePreviewConn) Close() error { return nil }

func (c *stubMySQLWritePreviewConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("transactions not supported")
}

func (c *stubMySQLWritePreviewConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	stubMySQLWritePreviewQueries = append(stubMySQLWritePreviewQueries, query)
	return &stubMySQLWritePreviewRows{columns: []string{"estimatedAffectedRows"}, values: [][]driver.Value{{int64(137)}}}, nil
}

type stubMySQLWritePreviewRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *stubMySQLWritePreviewRows) Columns() []string { return r.columns }
func (r *stubMySQLWritePreviewRows) Close() error      { return nil }
func (r *stubMySQLWritePreviewRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func TestSQLAdapterPreviewWrite_MySQLDeleteWhereCountsBeforeExecution(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	preview, err := adapter.PreviewWrite(context.Background(), ds, "DELETE FROM rooms WHERE user_id = 'u1'", WritePreviewOptions{})
	if err != nil {
		t.Fatalf("PreviewWrite returned error: %v", err)
	}

	if preview.Operation != "delete" {
		t.Fatalf("operation = %q, want delete", preview.Operation)
	}
	if preview.TargetEntity != "rooms" {
		t.Fatalf("targetEntity = %q, want rooms", preview.TargetEntity)
	}
	if preview.EstimatedAffectedRows != 137 {
		t.Fatalf("estimatedAffectedRows = %d, want 137", preview.EstimatedAffectedRows)
	}
	if !preview.RequiresElevatedApproval {
		t.Fatal("expected preview to require elevated approval over default threshold")
	}
	if len(stubMySQLWritePreviewQueries) != 1 {
		t.Fatalf("query count = %d, want 1", len(stubMySQLWritePreviewQueries))
	}
	want := "SELECT COUNT(*) AS estimatedAffectedRows FROM rooms WHERE user_id = 'u1'"
	if strings.TrimSpace(stubMySQLWritePreviewQueries[0]) != want {
		t.Fatalf("preview query\n got: %s\nwant: %s", stubMySQLWritePreviewQueries[0], want)
	}
}

func TestSQLAdapterPreviewWrite_MySQLDeleteUsingUnsupported(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	_, err := adapter.PreviewWrite(context.Background(), ds, "DELETE FROM t1 USING t1 JOIN t2 ON t2.id = t1.t2_id WHERE t2.disabled = 1", WritePreviewOptions{})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("PreviewWrite error = %v, want ErrUnsupported", err)
	}
	if len(stubMySQLWritePreviewQueries) != 0 {
		t.Fatalf("DELETE USING must not run preview query, got %v", stubMySQLWritePreviewQueries)
	}
}

func TestSQLAdapterPreviewWrite_MySQLDeleteJoinUnsupported(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	_, err := adapter.PreviewWrite(context.Background(), ds, "DELETE FROM t1 JOIN t2 ON t2.id = t1.t2_id WHERE t2.disabled = 1", WritePreviewOptions{})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("PreviewWrite error = %v, want ErrUnsupported", err)
	}
	if len(stubMySQLWritePreviewQueries) != 0 {
		t.Fatalf("DELETE JOIN must not run preview query, got %v", stubMySQLWritePreviewQueries)
	}
}

func TestSQLAdapterPreviewWrite_MySQLDeleteSkipsModifiers(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	_, err := adapter.PreviewWrite(context.Background(), ds, "DELETE LOW_PRIORITY QUICK IGNORE FROM rooms WHERE user_id = 'u1'", WritePreviewOptions{})
	if err != nil {
		t.Fatalf("PreviewWrite returned error: %v", err)
	}

	want := "SELECT COUNT(*) AS estimatedAffectedRows FROM rooms WHERE user_id = 'u1'"
	if strings.TrimSpace(stubMySQLWritePreviewQueries[0]) != want {
		t.Fatalf("preview query\n got: %s\nwant: %s", stubMySQLWritePreviewQueries[0], want)
	}
}

func TestSQLAdapterPreviewWrite_MySQLDeletePartitionCommaIsSingleTable(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	_, err := adapter.PreviewWrite(context.Background(), ds, "DELETE FROM rooms PARTITION (p0, p1) WHERE user_id = 'u1'", WritePreviewOptions{})
	if err != nil {
		t.Fatalf("PreviewWrite returned error: %v", err)
	}

	want := "SELECT COUNT(*) AS estimatedAffectedRows FROM rooms PARTITION (p0, p1) WHERE user_id = 'u1'"
	if strings.TrimSpace(stubMySQLWritePreviewQueries[0]) != want {
		t.Fatalf("preview query\n got: %s\nwant: %s", stubMySQLWritePreviewQueries[0], want)
	}
}

func TestSQLAdapterPreviewWrite_MySQLUpdateWhereCountsBeforeExecution(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	preview, err := adapter.PreviewWrite(context.Background(), ds, "UPDATE rooms SET archived = 1 WHERE user_id = 'u1' LIMIT 10", WritePreviewOptions{
		ElevatedApprovalThreshold: 200,
	})
	if err != nil {
		t.Fatalf("PreviewWrite returned error: %v", err)
	}

	if preview.Operation != "update" {
		t.Fatalf("operation = %q, want update", preview.Operation)
	}
	if preview.TargetEntity != "rooms" {
		t.Fatalf("targetEntity = %q, want rooms", preview.TargetEntity)
	}
	if preview.EstimatedAffectedRows != 10 {
		t.Fatalf("estimatedAffectedRows = %d, want 10", preview.EstimatedAffectedRows)
	}
	if preview.RequiresElevatedApproval {
		t.Fatal("did not expect elevated approval below overridden threshold")
	}
	want := "SELECT COUNT(*) AS estimatedAffectedRows FROM rooms WHERE user_id = 'u1'"
	if strings.TrimSpace(stubMySQLWritePreviewQueries[0]) != want {
		t.Fatalf("preview query\n got: %s\nwant: %s", stubMySQLWritePreviewQueries[0], want)
	}
}

func TestSQLAdapterPreviewWrite_MySQLOverflowLimitDoesNotCapEstimate(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	preview, err := adapter.PreviewWrite(context.Background(), ds, "UPDATE rooms SET archived = 1 WHERE user_id = 'u1' LIMIT 18446744073709551615", WritePreviewOptions{})
	if err != nil {
		t.Fatalf("PreviewWrite returned error: %v", err)
	}

	if preview.EstimatedAffectedRows != 137 {
		t.Fatalf("estimatedAffectedRows = %d, want 137", preview.EstimatedAffectedRows)
	}
	if !preview.RequiresElevatedApproval {
		t.Fatal("overflowed LIMIT must not suppress elevated approval")
	}
}

func TestSQLAdapterPreviewWrite_MySQLUpdateWithCTEStartsAtUpdateToken(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}
	statement := "WITH target_ids AS (SELECT id FROM rooms WHERE user_id = 'u1') UPDATE rooms SET archived = 1 WHERE id IN (SELECT id FROM target_ids)"

	_, err := adapter.PreviewWrite(context.Background(), ds, statement, WritePreviewOptions{})
	if err != nil {
		t.Fatalf("PreviewWrite returned error: %v", err)
	}

	want := "WITH target_ids AS (SELECT id FROM rooms WHERE user_id = 'u1') SELECT COUNT(*) AS estimatedAffectedRows FROM rooms WHERE id IN (SELECT id FROM target_ids)"
	if strings.TrimSpace(stubMySQLWritePreviewQueries[0]) != want {
		t.Fatalf("preview query\n got: %s\nwant: %s", stubMySQLWritePreviewQueries[0], want)
	}
}

func TestSQLAdapterPreviewWrite_MySQLUpdateJoinUnsupported(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	_, err := adapter.PreviewWrite(context.Background(), ds, "UPDATE rooms JOIN users ON users.id = rooms.user_id SET rooms.archived = 1 WHERE users.disabled = 1", WritePreviewOptions{})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("PreviewWrite error = %v, want ErrUnsupported", err)
	}
	if len(stubMySQLWritePreviewQueries) != 0 {
		t.Fatalf("UPDATE JOIN must not run preview query, got %v", stubMySQLWritePreviewQueries)
	}
}

func TestSQLAdapterPreviewWrite_MySQLLimitZeroCapsEstimate(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	preview, err := adapter.PreviewWrite(context.Background(), ds, "DELETE FROM rooms WHERE user_id = 'u1' LIMIT 0", WritePreviewOptions{})
	if err != nil {
		t.Fatalf("PreviewWrite returned error: %v", err)
	}

	if preview.EstimatedAffectedRows != 0 {
		t.Fatalf("estimatedAffectedRows = %d, want 0", preview.EstimatedAffectedRows)
	}
	if preview.RequiresElevatedApproval {
		t.Fatal("LIMIT 0 must not require elevated approval")
	}
}

func TestSQLAdapterPreviewWrite_MySQLUpdateSkipsModifiers(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	_, err := adapter.PreviewWrite(context.Background(), ds, "UPDATE LOW_PRIORITY IGNORE rooms SET archived = 1 WHERE user_id = 'u1'", WritePreviewOptions{})
	if err != nil {
		t.Fatalf("PreviewWrite returned error: %v", err)
	}

	want := "SELECT COUNT(*) AS estimatedAffectedRows FROM rooms WHERE user_id = 'u1'"
	if strings.TrimSpace(stubMySQLWritePreviewQueries[0]) != want {
		t.Fatalf("preview query\n got: %s\nwant: %s", stubMySQLWritePreviewQueries[0], want)
	}
}

func TestSQLAdapterPreviewWrite_MySQLUpdateIndexHintCommaIsSingleTable(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	_, err := adapter.PreviewWrite(context.Background(), ds, "UPDATE rooms USE INDEX (idx_user, idx_archived) SET archived = 1 WHERE user_id = 'u1'", WritePreviewOptions{})
	if err != nil {
		t.Fatalf("PreviewWrite returned error: %v", err)
	}

	want := "SELECT COUNT(*) AS estimatedAffectedRows FROM rooms USE INDEX (idx_user, idx_archived) WHERE user_id = 'u1'"
	if strings.TrimSpace(stubMySQLWritePreviewQueries[0]) != want {
		t.Fatalf("preview query\n got: %s\nwant: %s", stubMySQLWritePreviewQueries[0], want)
	}
}

func TestSQLAdapterPreviewWrite_MySQLUpdateIndexHintForJoinIsSingleTable(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	_, err := adapter.PreviewWrite(context.Background(), ds, "UPDATE rooms FORCE INDEX FOR JOIN (idx_user) SET archived = 1 WHERE user_id = 'u1'", WritePreviewOptions{})
	if err != nil {
		t.Fatalf("PreviewWrite returned error: %v", err)
	}

	want := "SELECT COUNT(*) AS estimatedAffectedRows FROM rooms FORCE INDEX FOR JOIN (idx_user) WHERE user_id = 'u1'"
	if strings.TrimSpace(stubMySQLWritePreviewQueries[0]) != want {
		t.Fatalf("preview query\n got: %s\nwant: %s", stubMySQLWritePreviewQueries[0], want)
	}
}

func TestSQLAdapterPreviewWrite_MySQLUpdateIndexHintThenJoinStillUnsupported(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	_, err := adapter.PreviewWrite(context.Background(), ds, "UPDATE rooms FORCE INDEX FOR JOIN (idx_user) JOIN users ON users.id = rooms.user_id SET archived = 1 WHERE users.disabled = 1", WritePreviewOptions{})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("PreviewWrite error = %v, want ErrUnsupported", err)
	}
	if len(stubMySQLWritePreviewQueries) != 0 {
		t.Fatalf("UPDATE JOIN must not run preview query, got %v", stubMySQLWritePreviewQueries)
	}
}

func TestSQLAdapterPreviewWrite_MySQLExplainUpdateIsNotPreviewable(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	_, err := adapter.PreviewWrite(context.Background(), ds, "EXPLAIN UPDATE rooms SET archived = 1 WHERE user_id = 'u1'", WritePreviewOptions{})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("PreviewWrite error = %v, want ErrUnsupported", err)
	}
	if len(stubMySQLWritePreviewQueries) != 0 {
		t.Fatalf("EXPLAIN UPDATE must not run preview query, got %v", stubMySQLWritePreviewQueries)
	}
}

func TestSQLAdapterPreviewWrite_MySQLDeleteWithoutWhereNotPreviewable(t *testing.T) {
	registerStubMySQLWritePreviewDriver()
	stubMySQLWritePreviewQueries = nil
	adapter := newStubMySQLWritePreviewAdapter()
	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	if _, err := adapter.PreviewWrite(context.Background(), ds, "DELETE FROM rooms", WritePreviewOptions{}); err == nil {
		t.Fatal("expected DELETE without WHERE to be rejected by preview")
	}
	if len(stubMySQLWritePreviewQueries) != 0 {
		t.Fatalf("DELETE without WHERE must not run preview query, got %v", stubMySQLWritePreviewQueries)
	}
}

func newStubMySQLWritePreviewAdapter() *SQLAdapter {
	return &SQLAdapter{
		driver:  stubMySQLWritePreviewDriverName,
		dialect: "mysql",
		dsn: func(datasource.DataSource) (string, error) {
			return "stub", nil
		},
		pools: make(map[string]*sql.DB),
		byID:  make(map[string]string),
	}
}

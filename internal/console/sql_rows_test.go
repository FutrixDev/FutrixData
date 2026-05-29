package console

import "testing"

type fakeRows struct {
	columns []string
	rows    [][]any
	idx     int
}

func (f *fakeRows) Columns() ([]string, error) {
	return f.columns, nil
}

func (f *fakeRows) Next() bool {
	if f.idx >= len(f.rows) {
		return false
	}
	f.idx++
	return true
}

func (f *fakeRows) Scan(dest ...any) error {
	row := f.rows[f.idx-1]
	for i := range dest {
		ptr, ok := dest[i].(*any)
		if !ok {
			continue
		}
		if i < len(row) {
			*ptr = row[i]
		}
	}
	return nil
}

func (f *fakeRows) Err() error {
	return nil
}

func TestReadRowsWindow_StopsAtLimit(t *testing.T) {
	rows := &fakeRows{
		columns: []string{"id"},
		rows: [][]any{
			{1},
			{2},
			{3},
		},
	}

	columns, data, hasMore, err := readRowsWindow(rows, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(columns) != 1 || columns[0] != "id" {
		t.Fatalf("unexpected columns: %v", columns)
	}
	if len(data) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(data))
	}
	if !hasMore {
		t.Fatalf("expected hasMore true")
	}
}

func TestReadRowsWindow_ByteValues(t *testing.T) {
	rows := &fakeRows{
		columns: []string{"name"},
		rows: [][]any{
			{[]byte("alpha")},
		},
	}

	_, data, hasMore, err := readRowsWindow(rows, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasMore {
		t.Fatalf("expected hasMore false")
	}
	if got, ok := data[0]["name"].(string); !ok || got != "alpha" {
		t.Fatalf("expected string conversion, got %#v", data[0]["name"])
	}
}

func TestReadSQLRowsWindow_PreservesDuplicateColumnsInOrderedData(t *testing.T) {
	rows := &fakeRows{
		columns: []string{"id", "id", "email"},
		rows: [][]any{
			{1, 2, []byte("alpha@example.com")},
		},
	}

	batch, hasMore, err := readSQLRowsWindow(rows, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasMore {
		t.Fatalf("expected hasMore false")
	}
	if len(batch.ColumnNames) != 3 {
		t.Fatalf("expected 3 display columns, got %d", len(batch.ColumnNames))
	}
	if batch.ColumnNames[0] != "id" || batch.ColumnNames[1] != "id" || batch.ColumnNames[2] != "email" {
		t.Fatalf("unexpected display columns: %#v", batch.ColumnNames)
	}
	if len(batch.ColumnKeys) != 3 {
		t.Fatalf("expected 3 column keys, got %d", len(batch.ColumnKeys))
	}
	if batch.ColumnKeys[0] != "id" || batch.ColumnKeys[1] != "id__2" || batch.ColumnKeys[2] != "email" {
		t.Fatalf("unexpected column keys: %#v", batch.ColumnKeys)
	}
	if len(batch.RowValues) != 1 || len(batch.RowValues[0]) != 3 {
		t.Fatalf("unexpected row values: %#v", batch.RowValues)
	}
	if got := batch.RowValues[0][1]; got != 2 {
		t.Fatalf("expected second id value preserved, got %#v", got)
	}
	if got, ok := batch.Rows[0]["id__2"].(int); !ok || got != 2 {
		t.Fatalf("expected compatibility row to preserve second id, got %#v", batch.Rows[0]["id__2"])
	}
	if got, ok := batch.Rows[0]["email"].(string); !ok || got != "alpha@example.com" {
		t.Fatalf("expected []byte value converted to string, got %#v", batch.Rows[0]["email"])
	}
}

func TestReadSQLRowsWindow_AvoidsRealColumnAndSyntheticKeyCollisions(t *testing.T) {
	rows := &fakeRows{
		columns: []string{"id", "id__2", "id"},
		rows: [][]any{
			{1, 200, 9},
		},
	}

	batch, hasMore, err := readSQLRowsWindow(rows, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasMore {
		t.Fatalf("expected hasMore false")
	}
	if len(batch.ColumnKeys) != 3 {
		t.Fatalf("expected 3 column keys, got %d", len(batch.ColumnKeys))
	}
	if batch.ColumnKeys[0] != "id" || batch.ColumnKeys[1] != "id__2" || batch.ColumnKeys[2] != "id__3" {
		t.Fatalf("unexpected column keys: %#v", batch.ColumnKeys)
	}
	if got, ok := batch.Rows[0]["id"].(int); !ok || got != 1 {
		t.Fatalf("expected first id preserved, got %#v", batch.Rows[0]["id"])
	}
	if got, ok := batch.Rows[0]["id__2"].(int); !ok || got != 200 {
		t.Fatalf("expected real id__2 preserved, got %#v", batch.Rows[0]["id__2"])
	}
	if got, ok := batch.Rows[0]["id__3"].(int); !ok || got != 9 {
		t.Fatalf("expected duplicate id to use next unique suffix, got %#v", batch.Rows[0]["id__3"])
	}
}

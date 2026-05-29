package window

type RowWindow struct {
	limit   int
	rows    []map[string]any
	hasMore bool
}

func NewRowWindow(limit int) *RowWindow {
	if limit < 0 {
		limit = 0
	}
	return &RowWindow{limit: limit}
}

func (w *RowWindow) Push(row map[string]any) bool {
	if w.limit == 0 {
		w.hasMore = true
		return false
	}
	if len(w.rows) < w.limit {
		w.rows = append(w.rows, row)
		return true
	}
	w.hasMore = true
	return false
}

func (w *RowWindow) Rows() []map[string]any {
	return w.rows
}

func (w *RowWindow) HasMore() bool {
	return w.hasMore
}

package console

func pagingNextOffset(current int64, rows int) int64 {
	if rows < 0 {
		rows = 0
	}
	return current + int64(rows)
}

func pagingPrevOffset(current int64, pageSize int) int64 {
	if pageSize < 0 {
		pageSize = 0
	}
	updated := current - int64(pageSize)
	if updated < 0 {
		return 0
	}
	return updated
}

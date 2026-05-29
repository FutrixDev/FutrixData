package console

import "futrixdata/platform/internal/console/paging"

type sqlSortKey struct {
	Column string
	Desc   bool
}

func sqlSortKeysFromToken(keys []paging.SortKey) []sqlSortKey {
	if len(keys) == 0 {
		return nil
	}
	out := make([]sqlSortKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, sqlSortKey{Column: key.Field, Desc: key.Desc})
	}
	return out
}

func pagingSortKeys(keys []sqlSortKey) []paging.SortKey {
	if len(keys) == 0 {
		return nil
	}
	out := make([]paging.SortKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, paging.SortKey{Field: key.Column, Desc: key.Desc})
	}
	return out
}

func sqlFetchLimit(pageSize int, totalLimit int64, offset int64) int {
	if pageSize < 0 {
		pageSize = 0
	}
	if totalLimit < 0 {
		return pageSize + 1
	}
	if totalLimit == 0 {
		return 0
	}
	if offset < 0 {
		offset = 0
	}
	remaining := totalLimit - offset
	if remaining <= 0 {
		return 0
	}
	fetch := pageSize + 1
	if int64(fetch) > remaining {
		fetch = int(remaining)
	}
	return fetch
}

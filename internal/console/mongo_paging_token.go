package console

import "futrixdata/platform/internal/console/paging"

func mongoSortKeysFromToken(keys []paging.SortKey) []mongoSortKey {
	if len(keys) == 0 {
		return nil
	}
	out := make([]mongoSortKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, mongoSortKey{Field: key.Field, Desc: key.Desc})
	}
	return out
}

func pagingSortKeysFromMongo(keys []mongoSortKey) []paging.SortKey {
	if len(keys) == 0 {
		return nil
	}
	out := make([]paging.SortKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, paging.SortKey{Field: key.Field, Desc: key.Desc})
	}
	return out
}

func mongoBuildPagingToken(dsID, statement string, pageSize int, keys []mongoSortKey, startCursor []any, endCursor []any, direction Direction, totalLimit int64, offset int64) (string, error) {
	encodedStart, err := mongoEncodeCursorValues(startCursor)
	if err != nil {
		return "", err
	}
	encodedEnd, err := mongoEncodeCursorValues(endCursor)
	if err != nil {
		return "", err
	}
	token := paging.Token{
		Version:      1,
		DatasourceID: dsID,
		QueryHash:    pagingQueryHash(statement),
		PageSize:     pageSize,
		Limit:        totalLimit,
		Offset:       offset,
		Sort:         pagingSortKeysFromMongo(keys),
		StartCursor:  encodedStart,
		EndCursor:    encodedEnd,
		Direction:    direction,
	}
	return paging.Encode(token)
}

func mongoFetchLimit(pageSize int, totalLimit int64, offset int64) int {
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

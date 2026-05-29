package aichat

import (
	"context"
	"sync"
)

type datasourceCacheKey struct{}

type datasourceCache struct {
	mu    sync.Mutex
	items map[string]datasourceCacheItem
}

type datasourceCacheItem struct {
	value DatasourceSummary
	err   error
}

func withDatasourceCache(ctx context.Context) context.Context {
	if ctx == nil {
		return ctx
	}
	if ctx.Value(datasourceCacheKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, datasourceCacheKey{}, &datasourceCache{items: make(map[string]datasourceCacheItem)})
}

func (s *Service) getDatasourceCached(ctx context.Context, id string) (DatasourceSummary, error) {
	cache, _ := ctx.Value(datasourceCacheKey{}).(*datasourceCache)
	if cache == nil {
		return s.tools.GetDatasource(ctx, id)
	}

	cache.mu.Lock()
	item, ok := cache.items[id]
	cache.mu.Unlock()
	if ok {
		return item.value, item.err
	}

	value, err := s.tools.GetDatasource(ctx, id)
	cache.mu.Lock()
	cache.items[id] = datasourceCacheItem{value: value, err: err}
	cache.mu.Unlock()
	return value, err
}

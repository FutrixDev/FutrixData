package console

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"futrixdata/platform/internal/datasource"
)

type SQLAdapter struct {
	driver  string
	dialect string
	dsn     func(datasource.DataSource) (string, error)
	mu      sync.Mutex
	pools   map[string]*sql.DB
	byID    map[string]string
}

func NewMySQLAdapter() *SQLAdapter {
	return &SQLAdapter{
		driver:  "mysql",
		dialect: "mysql",
		dsn:     mysqlDSN,
		pools:   make(map[string]*sql.DB),
		byID:    make(map[string]string),
	}
}

func NewPostgresAdapter() *SQLAdapter {
	return &SQLAdapter{
		driver:  "postgres",
		dialect: "postgres",
		dsn:     postgresDSN,
		pools:   make(map[string]*sql.DB),
		byID:    make(map[string]string),
	}
}

func (a *SQLAdapter) TestConnection(ctx context.Context, ds datasource.DataSource) error {
	db, err := a.dbFor(ctx, ds)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

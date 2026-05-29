package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/diagnostics"
)

func (a *App) GetDiagnosticsSettings() (diagnostics.Settings, error) {
	if a == nil || a.diagnostics == nil {
		return diagnostics.Settings{}, nil
	}
	return a.diagnostics.Current()
}

func (a *App) SetDatasourceTimingLogEnabled(enabled bool) (diagnostics.Settings, error) {
	if a == nil || a.diagnostics == nil {
		return diagnostics.Settings{}, nil
	}
	settings, err := a.diagnostics.SetDatasourceTimingLogEnabled(enabled)
	if err != nil {
		return diagnostics.Settings{}, err
	}
	a.logInfof("source=settings event=datasource_timing_log_toggled enabled=%t", enabled)
	return settings, nil
}

type appDatasourceTimingStarter func(ctx context.Context, entrypoint string, ds datasource.DataSource, statement string, opts console.ExecuteOptions, approved bool) (context.Context, func(error))

func newAppDatasourceTimingStarter(store *diagnostics.Store, logger console.DatasourceTimingLogger) appDatasourceTimingStarter {
	return func(ctx context.Context, entrypoint string, ds datasource.DataSource, statement string, opts console.ExecuteOptions, approved bool) (context.Context, func(error)) {
		if ctx == nil {
			ctx = context.Background()
		}
		if store == nil || logger == nil || !store.DatasourceTimingLogEnabled() {
			return ctx, func(error) {}
		}
		trace := console.NewDatasourceTimingTrace(logger, console.NewDatasourceTimingMetadata(entrypoint, newDatasourceTimingRequestID(), ds, statement, opts, approved))
		ctx = console.WithDatasourceTimingTrace(ctx, trace)
		console.DatasourceTimingEvent(ctx, "start")
		return ctx, func(err error) {
			trace.Finish(console.DatasourceTimingStatus(err), console.DatasourceTimingErrorFields(err)...)
		}
	}
}

func (a *App) beginDatasourceTiming(ctx context.Context, entrypoint string, ds datasource.DataSource, statement string, opts console.ExecuteOptions, approved bool) (context.Context, func(error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	if a == nil {
		return ctx, func(error) {}
	}
	return newAppDatasourceTimingStarter(a.diagnostics, a.infoLog)(ctx, entrypoint, ds, statement, opts, approved)
}

func newDatasourceTimingRequestID() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

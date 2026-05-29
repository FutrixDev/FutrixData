package main

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/securefile"

	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) CurrentAuth() (auth.State, error) {
	if a.authService == nil {
		return auth.State{}, nil
	}
	return a.authService.Current(context.Background())
}

func (a *App) EnsureAuthenticated() (auth.State, error) {
	if a.authService == nil {
		return auth.State{}, nil
	}
	return a.authService.EnsureAuthenticated(context.Background())
}

func (a *App) StartAuthLogin(input auth.StartLoginInput) (auth.LoginStart, error) {
	if a.authService == nil {
		return auth.LoginStart{}, errors.New("auth service is not configured")
	}
	return a.authService.StartLogin(context.Background(), input)
}

func (a *App) PollAuthLogin() (auth.LoginPoll, error) {
	if a.authService == nil {
		return auth.LoginPoll{}, errors.New("auth service is not configured")
	}
	return a.authService.PollLogin(context.Background())
}

func (a *App) CompleteAuthLogin(code string) (auth.State, error) {
	if a.authService == nil {
		return auth.State{}, errors.New("auth service is not configured")
	}
	state, err := a.authService.CompleteAuthLogin(context.Background(), strings.TrimSpace(code))
	if err != nil {
		return state, err
	}
	a.encryptExistingStores()
	return state, nil
}

func (a *App) LogoutAuth() (auth.State, error) {
	if a.authService == nil {
		return auth.State{}, nil
	}
	return a.authService.Logout(context.Background())
}

func (a *App) ListAuthDevices() (auth.DeviceList, error) {
	if a.authService == nil {
		return auth.DeviceList{}, errors.New("auth service is not configured")
	}
	return a.authService.ListDevices(context.Background())
}

func (a *App) RemoveAuthDevice(deviceID string) (auth.DeviceList, error) {
	if a.authService == nil {
		return auth.DeviceList{}, errors.New("auth service is not configured")
	}
	return a.authService.RemoveDevice(context.Background(), strings.TrimSpace(deviceID))
}

func (a *App) onSecondInstanceLaunch(data options.SecondInstanceData) {
	a.handleLaunchArgs(data.Args)
}

func (a *App) handleLaunchArgs(args []string) {
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "futrix://") || strings.HasPrefix(lower, "futrixdata://") {
			a.handleOpenURL(trimmed)
			return
		}
	}
}

func (a *App) handleOpenURL(rawURL string) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		a.emitAuthError(err)
		return
	}
	if !strings.EqualFold(parsed.Scheme, "futrix") && !strings.EqualFold(parsed.Scheme, "futrixdata") {
		return
	}
	callbackTarget := strings.Trim(strings.ToLower(parsed.Host+parsed.Path), "/")
	if callbackTarget == "codex/connect" {
		a.handleCodexConnectURL(parsed)
		return
	}
	if callbackTarget == "auth/callback" {
		callbackTarget = "callback"
	}
	if callbackTarget != "callback" {
		return
	}
	if a.authService == nil {
		return
	}
	code := strings.TrimSpace(parsed.Query().Get("code"))
	if code == "" {
		a.emitAuthError(errors.New("missing authorization code"))
		return
	}
	go func() {
		next, err := a.authService.CompleteAuthLogin(context.Background(), code)
		if err != nil {
			a.emitAuthError(err)
			return
		}
		a.encryptExistingStores()
		a.emitAuthState(next)
	}()
}

func (a *App) handleCodexConnectURL(parsed *url.URL) {
	if strings.TrimSpace(a.cfg.DataPath) == "" {
		return
	}
	if a.emitEvent != nil {
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		a.emitEvent(ctx, "codex:connect-request", map[string]any{
			"source":       strings.TrimSpace(parsed.Query().Get("source")),
			"authorizeUrl": parsed.String(),
		})
	}
}

func (a *App) encryptExistingStores() {
	if securefile.Key() == nil {
		return
	}
	if a.store != nil {
		_ = a.store.Save()
	}
	if a.aiConfigStore != nil {
		_ = a.aiConfigStore.Save()
	}
	if a.historyStore != nil {
		_ = a.historyStore.Save()
	}
	if a.entityCache != nil {
		_ = a.entityCache.Save()
	}
	if a.redisDocs != nil {
		_ = a.redisDocs.Save()
	}
}

func (a *App) emitAuthState(state auth.State) {
	if a.emitEvent != nil {
		a.emitEvent(a.ctx, "auth:state", state)
		return
	}
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "auth:state", state)
	}
}

func (a *App) emitAuthError(err error) {
	if err == nil {
		return
	}
	message := err.Error()
	if a.emitEvent != nil {
		a.emitEvent(a.ctx, "auth:error", message)
		return
	}
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "auth:error", message)
	}
}

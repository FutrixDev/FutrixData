package main

import "futrixdata/platform/internal/version"

func (a *App) GetAppVersion() string {
	return version.Version
}

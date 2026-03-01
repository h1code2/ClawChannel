package main

import "context"

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) AppInfo() map[string]string {
	return map[string]string{
		"name":    "ClawChannel",
		"version": "0.1.0-wails",
	}
}

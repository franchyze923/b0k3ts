package app

import (
	"b0k3ts/configs"
)

type App struct {
	Config configs.ServerConfig
}

func New() *App {

	return &App{
		Config: configs.ServerConfig{},
	}
}

func (app *App) Stop() {

}

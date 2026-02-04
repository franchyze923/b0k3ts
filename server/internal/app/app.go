package app

import (
	"b0k3ts/configs"
	"b0k3ts/internal/pkg/badger"
	"log/slog"
	"os"

	badgerDB "github.com/dgraph-io/badger/v4"
	"go.yaml.in/yaml/v4"
)

type App struct {
	Config   configs.ServerConfig
	BadgerDB *badgerDB.DB
}

func New() *App {

	return &App{
		Config: configs.ServerConfig{},
	}
}

func (app *App) Stop() {

}

func (app *App) Preflight() {

	// Initialize Badger
	//
	app.BadgerDB = badger.InitializeDatabase()

	// Load Server Config
	//
	file, err := os.ReadFile("config.yaml")
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	err = yaml.Unmarshal(file, &app.Config)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	// Saving Config on Badger
	//
	err = badger.PutKV(app.BadgerDB, "config", file)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	// Configuring Log Level
	//
	var programLevel slog.LevelVar
	programLevel.Set(slog.LevelDebug)

	// Configure HandlerOptions with the LevelVar.
	//
	handlerOptions := &slog.HandlerOptions{
		Level: &programLevel, // Use the address of the LevelVar
	}

	// Create the logger using a handler (e.g., TextHandler) and set as default.
	logger := slog.New(slog.NewTextHandler(os.Stdout, handlerOptions))
	slog.SetDefault(logger)
}

package app

import (
	"b0k3ts/configs"
	"b0k3ts/internal/pkg/auth"
	badgerDB "b0k3ts/internal/pkg/badger"
	"log/slog"
	"os"

	"github.com/dgraph-io/badger/v4"
	"go.yaml.in/yaml/v4"
)

type App struct {
	Config   configs.ServerConfig
	BadgerDB *badger.DB
}

func New() *App {

	// Initialize Badger
	//
	db := badgerDB.InitializeDatabase()

	return &App{
		Config:   configs.ServerConfig{},
		BadgerDB: db,
	}
}

func (app *App) Preflight() {

	// Create default user
	store := auth.NewStore(app.BadgerDB)

	created, err := store.EnsureUser("root", "b0k3ts", true)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	if !created {
		slog.Error("skipping default user creation, user already exists")
	}

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
	err = badgerDB.PutKV(app.BadgerDB, "config", file)
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

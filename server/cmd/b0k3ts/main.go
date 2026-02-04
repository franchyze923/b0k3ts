package main

import (
	"b0k3ts/internal/app"
	"log/slog"
	"os"

	"go.yaml.in/yaml/v4"
)

func main() {

	// 1. Create a LevelVar to control the log level dynamically.
	var programLevel slog.LevelVar
	programLevel.Set(slog.LevelDebug) // Set the initial level (e.g., INFO)

	// 2. Configure HandlerOptions with the LevelVar.
	handlerOptions := &slog.HandlerOptions{
		Level: &programLevel, // Use the address of the LevelVar
	}

	// 3. Create the logger using a handler (e.g., TextHandler) and set as default.
	logger := slog.New(slog.NewTextHandler(os.Stdout, handlerOptions))
	slog.SetDefault(logger)

	b0k3ts := app.New()

	file, err := os.ReadFile("config.yaml")
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	err = yaml.Unmarshal(file, &b0k3ts.Config)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)

	}

	b0k3ts.Serve()
}

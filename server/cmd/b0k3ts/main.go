package main

import (
	"b0k3ts/internal/app"
)

func main() {

	b0k3ts := app.New()

	b0k3ts.Preflight()

	b0k3ts.Serve()
}

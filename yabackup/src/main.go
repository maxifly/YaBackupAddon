package main

import (
	"os"
	"ybg/internal/appybg"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8099"
	}

	app := appybg.NewYbg(port)

	defer app.Stop()

	app.Start()
}

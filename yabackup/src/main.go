package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"ybg/internal/appybg"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8099"
	}

	app := appybg.NewYbg(port)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		app.Start()
	}()

	// Ждем сигнала остановки
	<-sigChan
	log.Println("Signal stop resieved")

	// Вызываем Stop() вручную (defer в main может не сработать при SIGKILL)
	app.Stop()

}

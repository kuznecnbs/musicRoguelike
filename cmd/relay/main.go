package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"soundcheck/internal/relay"
)

func main() {
	listen := flag.String("listen", ":7778", "TCP address for the relay server to listen on")
	flag.Parse()

	rl := relay.New(*listen)
	if err := rl.Start(); err != nil {
		log.Fatalf("failed to start relay on %s: %v", *listen, err)
	}
	log.Printf("Soundcheck relay listening on %s", rl.Addr())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down relay...")
	rl.Stop()
}

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/andywolf/agentium/internal/controller"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Agentium Controller starting")

	// Load session config from environment or file
	config, err := controller.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create controller
	ctrl, err := controller.New(config)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Received signal: %v", sig)
		cancel()
	}()

	// Run controller
	if err := ctrl.Run(ctx); err != nil {
		log.Printf("Controller exited with error: %v", err)
		os.Exit(1)
	}

	log.Println("Controller completed successfully")
}

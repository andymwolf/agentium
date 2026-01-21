package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	config, err := loadConfig()
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

func loadConfig() (controller.SessionConfig, error) {
	var config controller.SessionConfig

	// Try environment variable first
	if configJSON := os.Getenv("AGENTIUM_SESSION_CONFIG"); configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return config, fmt.Errorf("failed to parse AGENTIUM_SESSION_CONFIG: %w", err)
		}
		return config, nil
	}

	// Try config file
	configPath := os.Getenv("AGENTIUM_CONFIG_PATH")
	if configPath == "" {
		configPath = "/etc/agentium/session.json"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return config, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

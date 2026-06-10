// Package main demonstrates how to load configuration from a file and
// optionally watch for live changes using the config/file plugin.
//
// It creates a temporary JSON config file, loads it, prints the values, then
// watches for modifications (edit the file to see the update printed live).
//
// Run:
//
//	go run ./examples/config-basic
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tx7do/go-wind-plugins/config/file"
)

// appConfig is the application configuration struct.
type appConfig struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Debug    bool   `json:"debug"`
	Database struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
	} `json:"database"`
}

func main() {
	// ---------------------------------------------------------------
	// 1. Create a temporary config file for the demo
	// ---------------------------------------------------------------
	configPath := "./demo-config.json"
	initialConfig := appConfig{}
	initialConfig.Name = "my-service"
	initialConfig.Port = 8080
	initialConfig.Debug = true
	initialConfig.Database.Host = "localhost"
	initialConfig.Database.Port = 5432
	initialConfig.Database.Username = "admin"

	data, _ := json.MarshalIndent(initialConfig, "", "  ")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write config file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(configPath)

	fmt.Printf("Created config file: %s\n", configPath)
	fmt.Println(string(data))

	// ---------------------------------------------------------------
	// 2. Load config from file (one-shot read)
	// ---------------------------------------------------------------
	src, err := file.New(
		file.WithPath(configPath),
		file.WithWatch(true), // enable file watching for live reload
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create file source: %v\n", err)
		os.Exit(1)
	}
	defer src.Close()

	// One-shot load
	raw, err := src.Load(context.Background(), "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	var cfg appConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to decode config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nLoaded config: name=%s port=%d debug=%v db_host=%s\n",
		cfg.Name, cfg.Port, cfg.Debug, cfg.Database.Host)

	// ---------------------------------------------------------------
	// 3. Watch for changes (live reload)
	// ---------------------------------------------------------------
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := src.WatchValue(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to watch config: %v\n", err)
		os.Exit(1)
	}

	// Watcher goroutine — prints updated config when the file changes.
	go func() {
		for newData := range ch {
			var newCfg appConfig
			if err := json.Unmarshal(newData, &newCfg); err != nil {
				fmt.Fprintf(os.Stderr, "failed to decode updated config: %v\n", err)
				continue
			}
			fmt.Printf("\n[Config Updated] name=%s port=%d debug=%v db_host=%s\n",
				newCfg.Name, newCfg.Port, newCfg.Debug, newCfg.Database.Host)
		}
	}()

	// ---------------------------------------------------------------
	// 4. Auto-update the config after 2 seconds to demo live reload
	// ---------------------------------------------------------------
	go func() {
		time.Sleep(2 * time.Second)
		initialConfig.Port = 9090
		initialConfig.Debug = false
		initialConfig.Database.Host = "prod-db.example.com"
		updated, _ := json.MarshalIndent(initialConfig, "", "  ")
		_ = os.WriteFile(configPath, updated, 0644)
		fmt.Println("\nConfig file updated! Watcher should detect the change...")
	}()

	fmt.Println("\nWatching for config changes... (Ctrl+C to exit)")

	// Wait for signal.
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()
	cancel()
	fmt.Println("shutting down")
}

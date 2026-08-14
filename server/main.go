package main

import (
	"fmt"
	"os"

	"ollmo/ollmo/internal/config"
	"ollmo/ollmo/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfg, err := config.Load("config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "api":
		if err := server.RunAPI(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "api server: %v\n", err)
			os.Exit(1)
		}
	case "worker":
		if err := server.RunWorker(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "worker: %v\n", err)
			os.Exit(1)
		}
	case "migrate":
		if err := server.RunMigrate(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`ollmo - RAG platform

Usage:
  ollmo api       Run Fiber API server
  ollmo worker    Run Asynq worker
  ollmo migrate   Apply database schema migrations`)
}

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fractalmind-ai/fractalbot/internal/config"
	"github.com/fractalmind-ai/fractalbot/internal/gateway"
)

func main() {
	os.Exit(run())
}

func run() int {
	configPath := flag.String("config", "./config.yaml", "path to config file")
	portOverride := flag.Int("port", 0, "override gateway port")
	verbose := flag.Bool("verbose", false, "enable verbose logging")
	flag.Parse()

	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("failed to load config: %v", err)
		return 1
	}

	if cfg.Gateway == nil {
		cfg.Gateway = &config.GatewayConfig{Bind: "127.0.0.1", Port: 18789}
	}
	if *portOverride > 0 {
		cfg.Gateway.Port = *portOverride
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server, err := gateway.NewServer(cfg)
	if err != nil {
		log.Printf("failed to initialize gateway: %v", err)
		return 1
	}

	if err := server.Start(ctx); err != nil {
		log.Printf("gateway error: %v", err)
		if err := server.Stop(); err != nil {
			log.Printf("gateway shutdown error: %v", err)
		}
		return 1
	}

	if err := server.Stop(); err != nil {
		log.Printf("gateway shutdown error: %v", err)
		return 1
	}

	return 0
}

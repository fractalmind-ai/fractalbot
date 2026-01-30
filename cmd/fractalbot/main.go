package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fractalmind-ai/fractalbot/internal/config"
	"github.com/fractalmind-ai/fractalbot/internal/gateway"
)

func main() {
	os.Exit(Run(os.Args[1:], os.Stderr))
}

// Run executes the fractalbot CLI.
func Run(args []string, out io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return runWithContext(ctx, args, out)
}

func runWithContext(ctx context.Context, args []string, out io.Writer) int {
	fs := flag.NewFlagSet("fractalbot", flag.ContinueOnError)
	fs.SetOutput(out)

	configPath := fs.String("config", "./config.yaml", "path to config file")
	portOverride := fs.Int("port", 0, "override gateway port")
	verbose := fs.Bool("verbose", false, "enable verbose logging")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	logger := log.New(out, "", log.LstdFlags)
	if *verbose {
		logger.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		logger.Printf("failed to load config: %v", err)
		return 1
	}

	if cfg.Gateway == nil {
		cfg.Gateway = &config.GatewayConfig{Bind: "127.0.0.1", Port: 18789}
	}
	if *portOverride > 0 {
		cfg.Gateway.Port = *portOverride
	}

	server, err := gateway.NewServer(cfg)
	if err != nil {
		logger.Printf("failed to initialize gateway: %v", err)
		return 1
	}

	if err := server.Start(ctx); err != nil {
		logger.Printf("gateway error: %v", err)
		if err := server.Stop(); err != nil {
			logger.Printf("gateway shutdown error: %v", err)
		}
		return 1
	}

	if err := server.Stop(); err != nil {
		logger.Printf("gateway shutdown error: %v", err)
		return 1
	}

	return 0
}

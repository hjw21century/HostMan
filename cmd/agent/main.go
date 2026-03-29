package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"hostman/internal/agent"
)

func main() {
	var (
		server     = flag.String("server", "", "server URL (e.g. http://your-server:8080)")
		apiKey     = flag.String("key", "", "API key for this host")
		interval   = flag.Duration("interval", 60*time.Second, "reporting interval")
		noSvc      = flag.Bool("no-services", false, "skip service collection")
		insecure   = flag.Bool("insecure", false, "skip TLS certificate verification (for self-signed certs)")
		configPath = flag.String("config", "", "config file path (default: /etc/hostman/agent.json)")
		logFile    = flag.String("log", "", "log file path (default: stdout)")
		version    = flag.Bool("version", false, "show version")
	)
	flag.Parse()

	if *version {
		fmt.Println("hostman-agent v0.1.0")
		os.Exit(0)
	}

	// Try config file first, then CLI flags
	var cfg *agent.Config

	if *configPath != "" || (*server == "" && *apiKey == "") {
		// Load from config file
		loaded, err := agent.LoadConfig(*configPath)
		if err != nil {
			if *server == "" || *apiKey == "" {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
				fmt.Fprintln(os.Stderr, "Usage:")
				fmt.Fprintln(os.Stderr, "  hostman-agent -server URL -key API_KEY [-interval 60s]")
				fmt.Fprintln(os.Stderr, "  hostman-agent -config /path/to/config.json")
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "Config file format (/etc/hostman/agent.json):")
				fmt.Fprintln(os.Stderr, `  {`)
				fmt.Fprintln(os.Stderr, `    "server": "http://your-server:8080",`)
				fmt.Fprintln(os.Stderr, `    "api_key": "your-api-key",`)
				fmt.Fprintln(os.Stderr, `    "interval_sec": 60`)
				fmt.Fprintln(os.Stderr, `  }`)
				os.Exit(1)
			}
		} else {
			cfg = loaded
		}
	}

	// CLI flags override config file
	if cfg == nil {
		cfg = &agent.Config{}
	}
	if *server != "" {
		cfg.Server = *server
	}
	if *apiKey != "" {
		cfg.APIKey = *apiKey
	}
	if *interval != 60*time.Second || cfg.IntervalSec == 0 {
		cfg.IntervalSec = int(interval.Seconds())
	}
	if *noSvc {
		cfg.NoServices = true
	}
	if *insecure {
		cfg.Insecure = true
	}
	if *logFile != "" {
		cfg.LogFile = *logFile
	}

	if cfg.Server == "" || cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "Error: server and api_key are required")
		os.Exit(1)
	}

	// Setup logger
	logger := setupLogger(cfg.LogFile)

	logger.Println("🖥️  HostMan Agent v0.1.0")
	logger.Printf("   Server:   %s", cfg.Server)
	logger.Printf("   Interval: %ds", cfg.IntervalSec)
	logger.Printf("   Services: %v", !cfg.NoServices)

	// Print host info
	info := agent.CollectHostInfo()
	logger.Printf("   Host:     %s (%s %s)", info.Hostname, info.Distro, info.Kernel)
	logger.Printf("   CPU:      %s (%d cores)", info.CPUModel, info.CPUCores)
	logger.Println()

	agent.RunLoop(cfg, logger)
}

func setupLogger(logFile string) *log.Logger {
	if logFile == "" {
		return log.New(os.Stdout, "", log.LstdFlags)
	}

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open log file %s: %v, falling back to stdout\n", logFile, err)
		return log.New(os.Stdout, "", log.LstdFlags)
	}

	// Write to both file and stdout
	multi := io.MultiWriter(os.Stdout, f)
	return log.New(multi, "", log.LstdFlags)
}

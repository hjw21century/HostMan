package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hostman/internal/model"
)

// Reporter handles sending data to the server.
type Reporter struct {
	ServerURL string
	APIKey    string
	Client    *http.Client
	Logger    *log.Logger
}

// NewReporter creates a Reporter with sensible defaults.
func NewReporter(serverURL, apiKey string, logger *log.Logger) *Reporter {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}
	return &Reporter{
		ServerURL: serverURL,
		APIKey:    apiKey,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
		Logger: logger,
	}
}

// Report sends a metric + services + hostinfo + detail report to the server.
func (r *Reporter) Report(metric *model.Metric, services []model.Service, hostInfo *model.HostInfo, detail *model.DetailedData) error {
	report := model.AgentReport{
		Metric:   *metric,
		Services: services,
		HostInfo: hostInfo,
		Detail:   detail,
	}

	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	url := r.ServerURL + "/api/v1/report"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", r.APIKey)

	resp, err := r.Client.Do(req)
	if err != nil {
		return fmt.Errorf("send report: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// RunLoop starts the collection and reporting loop with graceful shutdown.
func RunLoop(cfg *Config, logger *log.Logger) {
	reporter := NewReporter(cfg.Server, cfg.APIKey, logger)
	interval := time.Duration(cfg.IntervalSec) * time.Second
	collectSvc := !cfg.NoServices

	logger.Printf("🔄 Agent starting — reporting to %s every %s", cfg.Server, interval)

	// Collect static host info (sent with first report only, then periodically)
	hostInfo := collectHostInfoModel()

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Printf("⏹️  Received %s, shutting down...", sig)
		cancel()
	}()

	// Immediate first report (with host info)
	doReport(reporter, collectSvc, hostInfo, logger)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	reportCount := 0
	for {
		select {
		case <-ctx.Done():
			logger.Println("👋 Agent stopped")
			return
		case <-ticker.C:
			reportCount++
			// Send host info every 100 reports (~every ~100 min at 60s interval)
			var info *model.HostInfo
			if reportCount%100 == 0 {
				info = collectHostInfoModel()
			}
			doReport(reporter, collectSvc, info, logger)
		}
	}
}

func doReport(reporter *Reporter, collectServices bool, hostInfo *model.HostInfo, logger *log.Logger) {
	metric, err := CollectMetric()
	if err != nil {
		logger.Printf("❌ collect error: %v", err)
		return
	}

	var services []model.Service
	if collectServices {
		services = CollectServices()
	}

	// Collect detailed data (disks, interfaces, top processes)
	detail := CollectDetailedData()

	if err := reporter.Report(metric, services, hostInfo, detail); err != nil {
		logger.Printf("❌ report error: %v", err)
		return
	}

	logger.Printf("✅ CPU: %.1f%% | Mem: %s/%s | Disk: %s/%s | Load: %.2f | Svc: %d",
		metric.CPUPercent,
		fmtB(metric.MemUsed), fmtB(metric.MemTotal),
		fmtB(metric.DiskUsed), fmtB(metric.DiskTotal),
		metric.Load1,
		len(services),
	)
}

func collectHostInfoModel() *model.HostInfo {
	info := CollectHostInfo()
	return &model.HostInfo{
		Hostname: info.Hostname,
		OS:       info.OS,
		Arch:     info.Arch,
		Kernel:   info.Kernel,
		Distro:   info.Distro,
		CPUModel: info.CPUModel,
		CPUCores: info.CPUCores,
	}
}

func fmtB(b uint64) string {
	const GB = 1024 * 1024 * 1024
	const MB = 1024 * 1024
	if b >= GB {
		return fmt.Sprintf("%.1fG", float64(b)/float64(GB))
	}
	return fmt.Sprintf("%.0fM", float64(b)/float64(MB))
}

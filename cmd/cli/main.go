package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

type Config struct {
	Server   string `json:"server"`
	Token    string `json:"token"`
	Insecure bool   `json:"insecure"`
}

type Host struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	IP           string  `json:"ip"`
	Provider     string  `json:"provider"`
	Plan         string  `json:"plan"`
	Cost         float64 `json:"cost"`
	Currency     string  `json:"currency"`
	BillingCycle string  `json:"billing_cycle"`
	Status       string  `json:"status"`
	ExpireAt     *string `json:"expire_at"`
	Note         string  `json:"note"`
}

type Metric struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemUsed    int64   `json:"mem_used"`
	MemTotal   int64   `json:"mem_total"`
	DiskUsed   int64   `json:"disk_used"`
	DiskTotal  int64   `json:"disk_total"`
	Load1      float64 `json:"load1"`
	Uptime     int64   `json:"uptime"`
}

type HostDetail struct {
	Host
	Metric *Metric `json:"metric"`
}

type StatusResp struct {
	TotalHosts   int     `json:"total_hosts"`
	OnlineHosts  int     `json:"online_hosts"`
	OfflineHosts int     `json:"offline_hosts"`
	ActiveAlerts int     `json:"active_alerts"`
	MonthlyCost  float64 `json:"monthly_cost"`
}

type Alert struct {
	ID        int64  `json:"id"`
	HostID    int64  `json:"host_id"`
	HostName  string `json:"host_name"`
	Type      string `json:"type"`
	Message   string `json:"message"`
	Resolved  bool   `json:"resolved"`
	CreatedAt string `json:"created_at"`
}

const usage = `HostMan CLI — 主机管理命令行工具

用法:
  hostman-cli <命令> [参数]

命令:
  config                配置服务器地址和API Token
  status                仪表板概览
  list                  列出所有主机
  show <ID|名称>        查看主机详情
  alerts                查看活跃告警
  export [csv|json]     导出主机信息

配置文件: ~/.hostman-cli.json

示例:
  hostman-cli config
  hostman-cli list
  hostman-cli show racknerd-639384
  hostman-cli alerts
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(0)
	}

	cmd := os.Args[1]
	switch cmd {
	case "config":
		cmdConfig()
	case "status":
		cmdStatus()
	case "list", "ls":
		cmdList()
	case "show", "info":
		if len(os.Args) < 3 {
			fatal("用法: hostman-cli show <ID|名称>")
		}
		cmdShow(os.Args[2])
	case "alerts":
		cmdAlerts()
	case "export":
		format := "csv"
		if len(os.Args) > 2 {
			format = os.Args[2]
		}
		cmdExport(format)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		fmt.Print(usage)
		os.Exit(1)
	}
}

func cmdConfig() {
	cfg := loadConfig()
	fmt.Printf("当前配置:\n  服务器: %s\n  Token: %s\n  跳过TLS: %v\n\n", cfg.Server, maskToken(cfg.Token), cfg.Insecure)

	fmt.Print("服务器地址 (留空保持不变): ")
	var server string
	fmt.Scanln(&server)
	if server != "" {
		cfg.Server = strings.TrimRight(server, "/")
	}

	fmt.Print("API Token (留空保持不变): ")
	var token string
	fmt.Scanln(&token)
	if token != "" {
		cfg.Token = token
	}

	fmt.Print("跳过TLS验证 (y/n, 留空保持不变): ")
	var insecure string
	fmt.Scanln(&insecure)
	if insecure == "y" || insecure == "Y" {
		cfg.Insecure = true
	} else if insecure == "n" || insecure == "N" {
		cfg.Insecure = false
	}

	saveConfig(cfg)
	fmt.Println("\n✅ 配置已保存")
}

func cmdStatus() {
	var resp StatusResp
	apiGet("/api/v1/admin/status", &resp)

	fmt.Println("📊 HostMan 仪表板")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("  主机总数:   %d\n", resp.TotalHosts)
	fmt.Printf("  在线:       \033[32m%d\033[0m\n", resp.OnlineHosts)
	fmt.Printf("  离线:       \033[31m%d\033[0m\n", resp.OfflineHosts)
	fmt.Printf("  活跃告警:   ")
	if resp.ActiveAlerts > 0 {
		fmt.Printf("\033[31m%d\033[0m\n", resp.ActiveAlerts)
	} else {
		fmt.Printf("\033[32m%d\033[0m\n", resp.ActiveAlerts)
	}
	fmt.Printf("  月均费用:   $%.2f\n", resp.MonthlyCost)
}

func cmdList() {
	var hosts []HostDetail
	apiGet("/api/v1/admin/hosts", &hosts)

	if len(hosts) == 0 {
		fmt.Println("暂无主机")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\t名称\tIP\t供应商\t状态\tCPU\t内存\t到期")
	fmt.Fprintln(w, "──\t────\t──\t────\t──\t───\t──\t──")

	for _, h := range hosts {
		status := colorStatus(h.Status)
		cpu := "-"
		mem := "-"
		if h.Metric != nil {
			cpu = fmt.Sprintf("%.1f%%", h.Metric.CPUPercent)
			if h.Metric.MemTotal > 0 {
				mem = fmt.Sprintf("%.0f%%", float64(h.Metric.MemUsed)/float64(h.Metric.MemTotal)*100)
			}
		}
		expire := "-"
		if h.ExpireAt != nil {
			if t, err := time.Parse(time.RFC3339, *h.ExpireAt); err == nil {
				days := int(time.Until(t).Hours() / 24)
				if days < 30 {
					expire = fmt.Sprintf("\033[33m%d天\033[0m", days)
				} else {
					expire = fmt.Sprintf("%d天", days)
				}
			} else {
				expire = *h.ExpireAt
			}
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			h.ID, h.Name, h.IP, h.Provider, status, cpu, mem, expire)
	}
	w.Flush()
}

func cmdShow(idOrName string) {
	// Try as ID first
	var host HostDetail
	id, err := strconv.Atoi(idOrName)
	if err == nil {
		apiGet(fmt.Sprintf("/api/v1/admin/hosts/%d", id), &host)
	} else {
		// Search by name
		var hosts []HostDetail
		apiGet("/api/v1/admin/hosts", &hosts)
		found := false
		for _, h := range hosts {
			if strings.EqualFold(h.Name, idOrName) || strings.Contains(strings.ToLower(h.Name), strings.ToLower(idOrName)) {
				host = h
				found = true
				break
			}
		}
		if !found {
			fatal("未找到主机: %s", idOrName)
		}
	}

	fmt.Printf("🖥️  %s\n", host.Name)
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("  ID:         %d\n", host.ID)
	fmt.Printf("  IP:         %s\n", host.IP)
	fmt.Printf("  供应商:     %s\n", host.Provider)
	fmt.Printf("  套餐:       %s\n", host.Plan)
	fmt.Printf("  费用:       %s %.2f/%s\n", host.Currency, host.Cost, host.BillingCycle)
	fmt.Printf("  状态:       %s\n", colorStatus(host.Status))
	if host.ExpireAt != nil {
		fmt.Printf("  到期:       %s\n", *host.ExpireAt)
	}
	if host.Note != "" {
		fmt.Printf("  备注:       %s\n", host.Note)
	}

	if host.Metric != nil {
		m := host.Metric
		fmt.Println()
		fmt.Println("  📊 资源使用")
		fmt.Printf("  CPU:        %.1f%%\n", m.CPUPercent)
		if m.MemTotal > 0 {
			fmt.Printf("  内存:       %s / %s (%.0f%%)\n",
				fmtBytes(m.MemUsed), fmtBytes(m.MemTotal),
				float64(m.MemUsed)/float64(m.MemTotal)*100)
		}
		if m.DiskTotal > 0 {
			fmt.Printf("  磁盘:       %s / %s (%.0f%%)\n",
				fmtBytes(m.DiskUsed), fmtBytes(m.DiskTotal),
				float64(m.DiskUsed)/float64(m.DiskTotal)*100)
		}
		fmt.Printf("  负载:       %.2f\n", m.Load1)
		if m.Uptime > 0 {
			fmt.Printf("  运行时间:   %s\n", fmtDuration(m.Uptime))
		}
	}
}

func cmdAlerts() {
	var alerts []Alert
	apiGet("/api/v1/admin/alerts", &alerts)

	if len(alerts) == 0 {
		fmt.Println("🎉 暂无活跃告警")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\t主机\t类型\t内容\t时间")
	fmt.Fprintln(w, "──\t──\t──\t──\t──")
	for _, a := range alerts {
		typeStr := a.Type
		switch a.Type {
		case "cpu":
			typeStr = "\033[31mCPU\033[0m"
		case "mem":
			typeStr = "\033[31m内存\033[0m"
		case "disk":
			typeStr = "\033[33m磁盘\033[0m"
		case "expire":
			typeStr = "\033[33m到期\033[0m"
		case "offline":
			typeStr = "\033[31m离线\033[0m"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", a.ID, a.HostName, typeStr, a.Message, a.CreatedAt)
	}
	w.Flush()
}

func cmdExport(format string) {
	cfg := loadConfig()
	url := cfg.Server + "/api/v1/admin/export?format=" + format
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	client := newHTTPClient(cfg)
	resp, err := client.Do(req)
	if err != nil {
		fatal("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fatal("API错误 (%d): %s", resp.StatusCode, string(body))
	}

	io.Copy(os.Stdout, resp.Body)
}

// ---- helpers ----

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hostman-cli.json")
}

func loadConfig() Config {
	var cfg Config
	data, err := os.ReadFile(configPath())
	if err == nil {
		json.Unmarshal(data, &cfg)
	}
	return cfg
}

func saveConfig(cfg Config) {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configPath(), data, 0600)
}

func newHTTPClient(cfg Config) *http.Client {
	client := &http.Client{Timeout: 15 * time.Second}
	if cfg.Insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return client
}

func apiGet(path string, out interface{}) {
	cfg := loadConfig()
	if cfg.Server == "" || cfg.Token == "" {
		fatal("请先运行 hostman-cli config 配置服务器和Token")
	}

	req, _ := http.NewRequest("GET", cfg.Server+path, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	client := newHTTPClient(cfg)
	resp, err := client.Do(req)
	if err != nil {
		fatal("连接失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		fatal("认证失败，请检查API Token")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fatal("API错误 (%d): %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		fatal("解析响应失败: %v", err)
	}
}

func colorStatus(s string) string {
	switch s {
	case "online":
		return "\033[32m● 在线\033[0m"
	case "offline":
		return "\033[31m● 离线\033[0m"
	default:
		return "\033[90m● 未知\033[0m"
	}
}

func maskToken(t string) string {
	if t == "" {
		return "(未设置)"
	}
	if len(t) > 8 {
		return t[:4] + "..." + t[len(t)-4:]
	}
	return "****"
}

func fmtBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.0f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.0f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func fmtDuration(secs int64) string {
	d := secs / 86400
	h := (secs % 86400) / 3600
	if d > 0 {
		return fmt.Sprintf("%d天%d小时", d, h)
	}
	m := (secs % 3600) / 60
	return fmt.Sprintf("%d小时%d分钟", h, m)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "❌ "+format+"\n", args...)
	os.Exit(1)
}

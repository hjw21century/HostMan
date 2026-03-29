package agent

import (
	"os"
	"runtime"
	"strings"
)

// HostInfo contains static host information sent on first report.
type HostInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`       // e.g. "linux"
	Arch     string `json:"arch"`     // e.g. "amd64"
	Kernel   string `json:"kernel"`   // e.g. "5.15.0-91-generic"
	Distro   string `json:"distro"`   // e.g. "Ubuntu 22.04.3 LTS"
	CPUModel string `json:"cpu_model"`
	CPUCores int    `json:"cpu_cores"`
}

// CollectHostInfo gathers static system information.
func CollectHostInfo() *HostInfo {
	info := &HostInfo{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
	}

	info.Hostname, _ = os.Hostname()
	info.Kernel = readFileTrimmed("/proc/sys/kernel/osrelease")
	info.Distro = parseDistro()
	info.CPUModel = parseCPUModel()

	return info
}

func readFileTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func parseDistro() string {
	// Try /etc/os-release first
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			val := strings.TrimPrefix(line, "PRETTY_NAME=")
			val = strings.Trim(val, "\"")
			return val
		}
	}
	return ""
}

func parseCPUModel() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

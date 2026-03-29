package agent

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"hostman/internal/model"
)

// CollectMetric gathers system resource metrics from /proc.
func CollectMetric() (*model.Metric, error) {
	m := &model.Metric{
		CollectedAt: time.Now(),
	}

	m.CPUPercent, _ = cpuPercent(500 * time.Millisecond)
	m.MemTotal, m.MemUsed, m.MemBuffered, m.MemCached, m.SwapTotal, m.SwapUsed, _ = memInfo()
	m.DiskTotal, m.DiskUsed, _ = diskUsage("/")
	m.NetIn, m.NetOut, _ = netIO()
	m.Load1, m.Load5, m.Load15, _ = loadAvg()
	m.Uptime, _ = uptime()
	m.ProcessCount = processCount()

	return m, nil
}

// CollectDetailedData gathers per-partition disk info, per-interface network info, and top processes.
func CollectDetailedData() *model.DetailedData {
	return &model.DetailedData{
		Disks:      collectDiskPartitions(),
		Interfaces: collectNetInterfaces(),
		TopCPU:     topProcesses("cpu", 5),
		TopMem:     topProcesses("mem", 5),
	}
}

// ---------- CPU ----------

func cpuPercent(interval time.Duration) (float64, error) {
	idle1, total1, err := cpuTimes()
	if err != nil {
		return 0, err
	}
	time.Sleep(interval)
	idle2, total2, err := cpuTimes()
	if err != nil {
		return 0, err
	}
	idleDelta := float64(idle2 - idle1)
	totalDelta := float64(total2 - total1)
	if totalDelta == 0 {
		return 0, nil
	}
	return (1.0 - idleDelta/totalDelta) * 100.0, nil
}

func cpuTimes() (idle, total uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0, 0, fmt.Errorf("unexpected /proc/stat format")
		}
		var vals []uint64
		for _, f := range fields[1:] {
			v, _ := strconv.ParseUint(f, 10, 64)
			vals = append(vals, v)
			total += v
		}
		if len(vals) >= 4 {
			idle = vals[3]
			if len(vals) >= 5 {
				idle += vals[4]
			}
		}
		return idle, total, nil
	}
	return 0, 0, fmt.Errorf("/proc/stat: cpu line not found")
}

// ---------- Memory ----------

func memInfo() (total, used, buffered, cached, swapTotal, swapUsed uint64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()

	var memTotal, memAvail, memBuf, memCache, swTotal, swFree uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(parts[1], 10, 64)
		val *= 1024 // kB to bytes
		switch parts[0] {
		case "MemTotal:":
			memTotal = val
		case "MemAvailable:":
			memAvail = val
		case "Buffers:":
			memBuf = val
		case "Cached:":
			memCache = val
		case "SwapTotal:":
			swTotal = val
		case "SwapFree:":
			swFree = val
		}
	}
	return memTotal, memTotal - memAvail, memBuf, memCache, swTotal, swTotal - swFree, nil
}

// ---------- Disk ----------

func diskUsage(path string) (total, used uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used = total - free
	return total, used, nil
}

func collectDiskPartitions() []model.DiskPartition {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer f.Close()

	var partitions []model.DiskPartition
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		device, mountPoint, fsType := fields[0], fields[1], fields[2]

		// Skip virtual filesystems
		if !strings.HasPrefix(device, "/dev/") {
			continue
		}
		// Skip snap and loop devices
		if strings.Contains(device, "loop") || strings.Contains(mountPoint, "/snap/") {
			continue
		}
		if seen[mountPoint] {
			continue
		}
		seen[mountPoint] = true

		var stat syscall.Statfs_t
		if err := syscall.Statfs(mountPoint, &stat); err != nil {
			continue
		}

		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bfree * uint64(stat.Bsize)
		if total == 0 {
			continue
		}

		partitions = append(partitions, model.DiskPartition{
			Device:     device,
			MountPoint: mountPoint,
			FSType:     fsType,
			Total:      total,
			Used:       total - free,
			Free:       free,
		})
	}

	// Sort by mount point
	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].MountPoint < partitions[j].MountPoint
	})
	return partitions
}

// ---------- Network ----------

func netIO() (rx, tx uint64, err error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum <= 2 {
			continue
		}
		line := scanner.Text()
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) < 10 {
			continue
		}
		iface := strings.TrimSuffix(parts[0], ":")
		if iface == "lo" {
			continue
		}
		r, _ := strconv.ParseUint(parts[1], 10, 64)
		t, _ := strconv.ParseUint(parts[9], 10, 64)
		rx += r
		tx += t
	}
	return rx, tx, nil
}

func collectNetInterfaces() []model.NetInterface {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil
	}
	defer f.Close()

	var ifaces []model.NetInterface
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum <= 2 {
			continue
		}
		line := scanner.Text()
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) < 10 {
			continue
		}
		name := strings.TrimSuffix(parts[0], ":")
		if name == "lo" {
			continue
		}
		rxBytes, _ := strconv.ParseUint(parts[1], 10, 64)
		rxPkts, _ := strconv.ParseUint(parts[2], 10, 64)
		txBytes, _ := strconv.ParseUint(parts[9], 10, 64)
		txPkts, _ := strconv.ParseUint(parts[10], 10, 64)

		ni := model.NetInterface{
			Name:    name,
			RxBytes: rxBytes,
			TxBytes: txBytes,
			RxPkts:  rxPkts,
			TxPkts:  txPkts,
		}

		// Get IPv4 address
		if iface, err := net.InterfaceByName(name); err == nil {
			if addrs, err := iface.Addrs(); err == nil {
				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
						ni.IPv4 = ipnet.IP.String()
						break
					}
				}
			}
		}
		ifaces = append(ifaces, ni)
	}
	return ifaces
}

// ---------- Load ----------

func loadAvg() (l1, l5, l15 float64, err error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, err
	}
	parts := strings.Fields(string(data))
	if len(parts) < 3 {
		return 0, 0, 0, fmt.Errorf("unexpected /proc/loadavg format")
	}
	l1, _ = strconv.ParseFloat(parts[0], 64)
	l5, _ = strconv.ParseFloat(parts[1], 64)
	l15, _ = strconv.ParseFloat(parts[2], 64)
	return l1, l5, l15, nil
}

// ---------- Uptime ----------

func uptime() (int64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	parts := strings.Fields(string(data))
	if len(parts) < 1 {
		return 0, fmt.Errorf("unexpected /proc/uptime format")
	}
	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}
	return int64(val), nil
}

// ---------- Process Count ----------

func processCount() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			if _, err := strconv.Atoi(e.Name()); err == nil {
				count++
			}
		}
	}
	return count
}

// ---------- Top Processes ----------

func topProcesses(sortBy string, limit int) []model.ProcessInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	var procs []model.ProcessInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		info := readProcessInfo(pid)
		if info != nil {
			procs = append(procs, *info)
		}
	}

	if sortBy == "cpu" {
		sort.Slice(procs, func(i, j int) bool { return procs[i].CPU > procs[j].CPU })
	} else {
		sort.Slice(procs, func(i, j int) bool { return procs[i].MemRSS > procs[j].MemRSS })
	}

	if len(procs) > limit {
		procs = procs[:limit]
	}
	return procs
}

func readProcessInfo(pid int) *model.ProcessInfo {
	// Read comm (process name)
	commData, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return nil
	}
	name := strings.TrimSpace(string(commData))

	// Read status for memory
	statusData, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return nil
	}

	var rss uint64
	for _, line := range strings.Split(string(statusData), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, _ := strconv.ParseUint(fields[1], 10, 64)
				rss = v * 1024 // kB to bytes
			}
			break
		}
	}

	// Calculate CPU% from /proc/[pid]/stat
	cpuPct := readProcessCPU(pid)

	// Get total memory for percentage
	memTotal, _, _, _, _, _, _ := memInfo()
	var memPct float64
	if memTotal > 0 {
		memPct = float64(rss) / float64(memTotal) * 100
	}

	return &model.ProcessInfo{
		PID:    pid,
		Name:   name,
		CPU:    cpuPct,
		Mem:    memPct,
		MemRSS: rss,
	}
}

func readProcessCPU(pid int) float64 {
	statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}

	// Find the closing paren (process name may contain spaces)
	s := string(statData)
	idx := strings.LastIndex(s, ")")
	if idx < 0 || idx+2 >= len(s) {
		return 0
	}
	fields := strings.Fields(s[idx+2:])
	if len(fields) < 12 {
		return 0
	}

	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)

	// Get system uptime
	uptimeSecs, _ := uptime()

	// Get clock ticks per second
	clkTck := uint64(100) // default on most Linux
	if out, err := exec.Command("getconf", "CLK_TCK").Output(); err == nil {
		if v, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64); err == nil {
			clkTck = v
		}
	}

	// Process start time (field 19, 0-indexed from after comm)
	startTime, _ := strconv.ParseUint(fields[19], 10, 64)

	totalTime := utime + stime
	seconds := float64(uptimeSecs) - float64(startTime)/float64(clkTck)
	if seconds <= 0 {
		return 0
	}

	return (float64(totalTime) / float64(clkTck) / seconds) * 100.0
}

// ---------- Services ----------

func CollectServices() []model.Service {
	var services []model.Service
	services = append(services, collectSystemd()...)
	services = append(services, collectDocker()...)
	return services
}

func collectSystemd() []model.Service {
	out, err := exec.Command("systemctl", "list-units", "--type=service", "--state=running,failed,exited",
		"--no-pager", "--no-legend", "--plain").Output()
	if err != nil {
		return nil
	}

	var services []model.Service
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ".service")
		if isSystemService(name) {
			continue
		}
		status := "unknown"
		switch fields[3] {
		case "running":
			status = "running"
		case "exited":
			status = "stopped"
		case "failed":
			status = "error"
		}
		services = append(services, model.Service{
			Name:      name,
			Type:      "systemd",
			Status:    status,
			CheckedAt: time.Now(),
		})
	}
	return services
}

func collectDocker() []model.Service {
	out, err := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}\t{{.State}}").Output()
	if err != nil {
		return nil
	}
	var services []model.Service
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 2)
		if len(parts) < 2 {
			continue
		}
		status := "unknown"
		switch parts[1] {
		case "running":
			status = "running"
		case "exited", "dead":
			status = "stopped"
		case "restarting", "paused":
			status = parts[1]
		}
		services = append(services, model.Service{
			Name:      parts[0],
			Type:      "docker",
			Status:    status,
			CheckedAt: time.Now(),
		})
	}
	return services
}

func isSystemService(name string) bool {
	skip := []string{
		"systemd-", "dbus", "polkit", "accounts-daemon", "networkd-dispatcher",
		"rsyslog", "cron", "atd", "getty", "serial-getty", "console-getty",
		"user@", "session-", "modprobe@", "kmod-static-nodes",
		"dev-hugepages", "dev-mqueue", "sys-", "proc-",
		"tmp.", "run-", "snap.", "snapd",
		"blk-availability", "lvm2", "dm-event",
		"multipathd", "plymouth",
	}
	lower := strings.ToLower(name)
	for _, s := range skip {
		if strings.HasPrefix(lower, s) || strings.Contains(lower, s) {
			return true
		}
	}
	// Also skip if path looks like /init.scope or similar
	if strings.HasSuffix(lower, ".scope") || strings.HasSuffix(lower, ".mount") {
		return true
	}
	_ = filepath.Base(name) // suppress unused import
	return false
}

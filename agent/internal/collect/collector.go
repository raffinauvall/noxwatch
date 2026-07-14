package collect

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type cpuCounters struct{ total, idle uint64 }

type networkCounters struct{ rx, tx int64 }

type Collector struct {
	procRoot    string
	previousCPU cpuCounters
	previousNet map[string]networkCounters
	previousAt  time.Time
}

func New() *Collector {
	return &Collector{procRoot: "/proc", previousNet: map[string]networkCounters{}}
}

func CollectIdentity(version string) (Identity, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return Identity{}, err
	}
	machine, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return Identity{}, fmt.Errorf("read machine id: %w", err)
	}
	osName, osVersion := osRelease()
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		return Identity{}, err
	}
	id := sha256.Sum256([]byte(strings.TrimSpace(string(machine))))
	return Identity{Hostname: hostname, MachineID: hex.EncodeToString(id[:]), OS: osName, OSVersion: osVersion,
		KernelVersion: chars(uname.Release[:]), Architecture: runtime.GOARCH, AgentVersion: version}, nil
}

func (c *Collector) Collect(serverID string, sequence int64) (Payload, error) {
	now := time.Now().UTC()
	cpuFile, err := os.Open(c.procRoot + "/stat")
	if err != nil {
		return Payload{}, err
	}
	counters, processes, zombies, err := parseProcStat(cpuFile)
	cpuFile.Close()
	if err != nil {
		return Payload{}, err
	}
	memFile, err := os.Open(c.procRoot + "/meminfo")
	if err != nil {
		return Payload{}, err
	}
	memory, swap, err := parseMemInfo(memFile)
	memFile.Close()
	if err != nil {
		return Payload{}, err
	}
	load, err := readLoad(c.procRoot + "/loadavg")
	if err != nil {
		return Payload{}, err
	}
	uptime, err := readUptime(c.procRoot + "/uptime")
	if err != nil {
		return Payload{}, err
	}
	networks, err := c.readNetworks(now)
	if err != nil {
		return Payload{}, err
	}
	disks, err := readDisks(c.procRoot + "/self/mounts")
	if err != nil {
		return Payload{}, err
	}
	cpu := CPUMetrics{UsagePercent: cpuUsage(c.previousCPU, counters), Load1: load[0], Load5: load[1], Load15: load[2], LogicalCPUCount: runtime.NumCPU(), PhysicalCPUCount: physicalCPUs(c.procRoot + "/cpuinfo")}
	c.previousCPU, c.previousAt = counters, now
	return Payload{ServerID: serverID, CollectedAt: now, Sequence: sequence,
		System: SystemMetrics{UptimeSeconds: uptime, ProcessCount: processes, ZombieProcessCount: zombies}, CPU: cpu,
		Memory: memory, Swap: swap, Disks: disks, Networks: networks}, nil
}

func parseProcStat(r io.Reader) (cpuCounters, int, int, error) {
	scanner := bufio.NewScanner(r)
	var counters cpuCounters
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "cpu":
			for index, raw := range fields[1:] {
				value, err := strconv.ParseUint(raw, 10, 64)
				if err != nil {
					return cpuCounters{}, 0, 0, err
				}
				counters.total += value
				if index == 3 || index == 4 {
					counters.idle += value
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return cpuCounters{}, 0, 0, err
	}
	processes, zombies := processCounts()
	return counters, processes, zombies, nil
}

func parseMemInfo(r io.Reader) (MemoryMetrics, SwapMetrics, error) {
	values := map[string]int64{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			value, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return MemoryMetrics{}, SwapMetrics{}, err
			}
			values[strings.TrimSuffix(fields[0], ":")] = value * 1024
		}
	}
	if err := scanner.Err(); err != nil {
		return MemoryMetrics{}, SwapMetrics{}, err
	}
	memory := MemoryMetrics{TotalBytes: values["MemTotal"], AvailableBytes: values["MemAvailable"]}
	memory.UsedBytes = max(0, memory.TotalBytes-memory.AvailableBytes)
	memory.UsagePercent = percent(memory.UsedBytes, memory.TotalBytes)
	swap := SwapMetrics{TotalBytes: values["SwapTotal"], UsedBytes: max(0, values["SwapTotal"]-values["SwapFree"])}
	swap.UsagePercent = percent(swap.UsedBytes, swap.TotalBytes)
	return memory, swap, nil
}

func (c *Collector) readNetworks(now time.Time) ([]NetworkMetrics, error) {
	file, err := os.Open(c.procRoot + "/net/dev")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	metrics, err := parseNetworks(file)
	if err != nil {
		return nil, err
	}
	seconds := now.Sub(c.previousAt).Seconds()
	for index := range metrics {
		current := &metrics[index]
		previous, ok := c.previousNet[current.Interface]
		if ok && seconds > 0 {
			current.RXBytesPerSecond = float64(max(0, current.RXBytesTotal-previous.rx)) / seconds
			current.TXBytesPerSecond = float64(max(0, current.TXBytesTotal-previous.tx)) / seconds
		}
		c.previousNet[current.Interface] = networkCounters{rx: current.RXBytesTotal, tx: current.TXBytesTotal}
	}
	return metrics, nil
}

func parseNetworks(r io.Reader) ([]NetworkMetrics, error) {
	result := []NetworkMetrics{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}
		value := func(index int) int64 { parsed, _ := strconv.ParseInt(fields[index], 10, 64); return parsed }
		result = append(result, NetworkMetrics{Interface: strings.TrimSpace(parts[0]), RXBytesTotal: value(0), RXPacketsTotal: value(1), RXErrorsTotal: value(2), TXBytesTotal: value(8), TXPacketsTotal: value(9), TXErrorsTotal: value(10)})
	}
	return result, scanner.Err()
}

func readDisks(path string) ([]DiskMetrics, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	excluded := map[string]bool{"proc": true, "sysfs": true, "tmpfs": true, "devtmpfs": true, "cgroup": true, "cgroup2": true, "overlay": true, "squashfs": true}
	result := []DiskMetrics{}
	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 || excluded[fields[2]] {
			continue
		}
		mount := unescapeMount(fields[1])
		if seen[mount] {
			continue
		}
		seen[mount] = true
		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount, &stat); err != nil {
			continue
		}
		total := int64(stat.Blocks) * int64(stat.Bsize)
		available := int64(stat.Bavail) * int64(stat.Bsize)
		used := total - int64(stat.Bfree)*int64(stat.Bsize)
		inodeUsed := int64(stat.Files - stat.Ffree)
		result = append(result, DiskMetrics{MountPoint: mount, Filesystem: fields[2], TotalBytes: total, UsedBytes: used, AvailableBytes: available, UsagePercent: percent(used, total), InodeUsagePercent: percent(inodeUsed, int64(stat.Files))})
	}
	return result, scanner.Err()
}

func cpuUsage(previous, current cpuCounters) float64 {
	if previous.total == 0 || current.total <= previous.total {
		return 0
	}
	total := current.total - previous.total
	idle := current.idle - previous.idle
	return float64(total-idle) * 100 / float64(total)
}

func readLoad(path string) ([3]float64, error) {
	var result [3]float64
	body, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	_, err = fmt.Sscan(string(body), &result[0], &result[1], &result[2])
	return result, err
}

func readUptime(path string) (int64, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var seconds float64
	if _, err := fmt.Sscan(string(body), &seconds); err != nil {
		return 0, err
	}
	return int64(seconds), nil
}

func processCounts() (int, int) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, 0
	}
	processes, zombies := 0, 0
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		processes++
		body, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
		if err == nil {
			end := strings.LastIndexByte(string(body), ')')
			if end >= 0 && len(body) > end+2 && body[end+2] == 'Z' {
				zombies++
			}
		}
	}
	return processes, zombies
}

func physicalCPUs(path string) int {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	cores := map[string]bool{}
	physical, core := "", ""
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		switch strings.TrimSpace(parts[0]) {
		case "physical id":
			physical = strings.TrimSpace(parts[1])
		case "core id":
			core = strings.TrimSpace(parts[1])
			cores[physical+":"+core] = true
		}
	}
	return len(cores)
}

func osRelease() (string, string) {
	body, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "linux", ""
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = strings.Trim(parts[1], `"`)
		}
	}
	return values["ID"], values["PRETTY_NAME"]
}

func chars(value []int8) string {
	bytes := make([]byte, 0, len(value))
	for _, char := range value {
		if char == 0 {
			break
		}
		bytes = append(bytes, byte(char))
	}
	return string(bytes)
}

func unescapeMount(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(value)
}

func percent(used, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(used) * 100 / float64(total)
}

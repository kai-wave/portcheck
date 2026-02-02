// portcheck - A simple CLI tool to check if ports are open/in use
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// PortResult holds the result of a port check
type PortResult struct {
	Port   int
	InUse  bool
	PID    int
	Process string
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	showPID := false
	portArg := os.Args[1]

	// Parse --pid flag
	if os.Args[1] == "--pid" || os.Args[1] == "-p" {
		if len(os.Args) < 3 {
			fmt.Println(colorRed + "Error: --pid requires a port number" + colorReset)
			os.Exit(1)
		}
		showPID = true
		portArg = os.Args[2]
	}

	// Check for help flag
	if portArg == "-h" || portArg == "--help" {
		printUsage()
		os.Exit(0)
	}

	// Parse port or range
	if strings.Contains(portArg, "-") {
		// Range: 8080-8090
		parts := strings.Split(portArg, "-")
		if len(parts) != 2 {
			fmt.Println(colorRed + "Error: Invalid port range format" + colorReset)
			os.Exit(1)
		}

		start, err1 := strconv.Atoi(parts[0])
		end, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || start > end || start < 1 || end > 65535 {
			fmt.Println(colorRed + "Error: Invalid port range" + colorReset)
			os.Exit(1)
		}

		checkPortRange(start, end, showPID)
	} else {
		// Single port
		port, err := strconv.Atoi(portArg)
		if err != nil || port < 1 || port > 65535 {
			fmt.Println(colorRed + "Error: Invalid port number" + colorReset)
			os.Exit(1)
		}

		result := checkPort(port, showPID)
		printResult(result, showPID)
	}
}

func printUsage() {
	fmt.Printf(`%s%sportcheck%s - Check if ports are open/in use

%sUsage:%s
  portcheck <port>           Check a single port
  portcheck <start>-<end>    Check a range of ports
  portcheck --pid <port>     Show process using the port

%sExamples:%s
  portcheck 8080             Check if port 8080 is in use
  portcheck 3000-3010        Scan ports 3000 through 3010
  portcheck --pid 22         Show what's using port 22

%sFlags:%s
  -p, --pid    Show process ID and name using the port
  -h, --help   Show this help message
`, colorBold, colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset)
}

// checkPort checks if a single port is in use
func checkPort(port int, getPID bool) PortResult {
	result := PortResult{Port: port}

	// Try to listen on the port to check if it's in use
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		result.InUse = true
		if getPID {
			result.PID, result.Process = findProcessByPort(port)
		}
	} else {
		listener.Close()
		result.InUse = false
	}

	return result
}

// checkPortRange scans a range of ports concurrently
func checkPortRange(start, end int, showPID bool) {
	var wg sync.WaitGroup
	results := make(chan PortResult, end-start+1)

	// Limit concurrency to avoid too many open files
	semaphore := make(chan struct{}, 100)

	fmt.Printf("%sScanning ports %d-%d...%s\n\n", colorCyan, start, end, colorReset)
	startTime := time.Now()

	for port := start; port <= end; port++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			result := checkPort(p, showPID)
			results <- result
		}(port)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and sort results
	portResults := make([]PortResult, 0, end-start+1)
	for result := range results {
		portResults = append(portResults, result)
	}

	// Sort by port number
	for i := 0; i < len(portResults)-1; i++ {
		for j := i + 1; j < len(portResults); j++ {
			if portResults[i].Port > portResults[j].Port {
				portResults[i], portResults[j] = portResults[j], portResults[i]
			}
		}
	}

	// Print results
	inUseCount := 0
	for _, result := range portResults {
		if result.InUse {
			inUseCount++
			printResult(result, showPID)
		}
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\n%s%d ports scanned in %v | %d in use, %d available%s\n",
		colorCyan, len(portResults), elapsed.Round(time.Millisecond),
		inUseCount, len(portResults)-inUseCount, colorReset)
}

// printResult displays a single port result
func printResult(result PortResult, showPID bool) {
	if result.InUse {
		status := fmt.Sprintf("%s●%s", colorRed, colorReset)
		info := fmt.Sprintf("Port %s%d%s is %s%sin use%s",
			colorBold, result.Port, colorReset,
			colorRed, colorBold, colorReset)

		if showPID && result.PID > 0 {
			info += fmt.Sprintf(" (PID: %s%d%s, Process: %s%s%s)",
				colorYellow, result.PID, colorReset,
				colorCyan, result.Process, colorReset)
		} else if showPID {
			info += fmt.Sprintf(" %s(process info unavailable - may need root)%s",
				colorYellow, colorReset)
		}

		fmt.Printf("%s %s\n", status, info)
	} else {
		status := fmt.Sprintf("%s○%s", colorGreen, colorReset)
		fmt.Printf("%s Port %s%d%s is %s%savailable%s\n",
			status, colorBold, result.Port, colorReset,
			colorGreen, colorBold, colorReset)
	}
}

// findProcessByPort finds the PID and process name using a port (Linux only)
func findProcessByPort(port int) (int, string) {
	// Read /proc/net/tcp and /proc/net/tcp6
	for _, netFile := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		if pid, name := searchNetFile(netFile, port); pid > 0 {
			return pid, name
		}
	}
	return 0, ""
}

// searchNetFile searches a /proc/net file for a port
func searchNetFile(path string, port int) (int, string) {
	file, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer file.Close()

	portHex := fmt.Sprintf("%04X", port)
	scanner := bufio.NewScanner(file)
	scanner.Scan() // Skip header

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}

		// local_address is field 1, format: IP:PORT in hex
		localAddr := fields[1]
		parts := strings.Split(localAddr, ":")
		if len(parts) != 2 {
			continue
		}

		// Check if port matches and socket is listening (state 0A)
		if parts[1] == portHex && fields[3] == "0A" {
			inode := fields[9]
			return findPIDByInode(inode)
		}
	}

	return 0, ""
}

// findPIDByInode finds PID by socket inode
func findPIDByInode(inode string) (int, string) {
	procDir, err := os.Open("/proc")
	if err != nil {
		return 0, ""
	}
	defer procDir.Close()

	entries, err := procDir.Readdirnames(-1)
	if err != nil {
		return 0, ""
	}

	socketLink := fmt.Sprintf("socket:[%s]", inode)

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry)
		if err != nil {
			continue // Not a PID directory
		}

		fdPath := filepath.Join("/proc", entry, "fd")
		fds, err := os.ReadDir(fdPath)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdPath, fd.Name()))
			if err != nil {
				continue
			}

			if link == socketLink {
				// Found it! Get process name
				commPath := filepath.Join("/proc", entry, "comm")
				comm, err := os.ReadFile(commPath)
				if err != nil {
					return pid, "unknown"
				}
				return pid, strings.TrimSpace(string(comm))
			}
		}
	}

	return 0, ""
}

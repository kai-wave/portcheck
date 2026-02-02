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

const (
	reset, red, green, yellow, cyan, bold = "\033[0m", "\033[31m", "\033[32m", "\033[33m", "\033[36m", "\033[1m"
)

type PortResult struct {
	Port    int
	InUse   bool
	PID     int
	Process string
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	showPID := false
	portArg := os.Args[1]

	if os.Args[1] == "--pid" || os.Args[1] == "-p" {
		if len(os.Args) < 3 {
			fmt.Println(red + "Error: --pid requires a port number" + reset)
			os.Exit(1)
		}
		showPID, portArg = true, os.Args[2]
	}

	if portArg == "-h" || portArg == "--help" {
		printUsage()
		os.Exit(0)
	}

	if strings.Contains(portArg, "-") {
		parts := strings.Split(portArg, "-")
		if len(parts) != 2 {
			fmt.Println(red + "Error: Invalid port range format" + reset)
			os.Exit(1)
		}
		start, err1 := strconv.Atoi(parts[0])
		end, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || start > end || start < 1 || end > 65535 {
			fmt.Println(red + "Error: Invalid port range" + reset)
			os.Exit(1)
		}
		checkPortRange(start, end, showPID)
	} else {
		port, err := strconv.Atoi(portArg)
		if err != nil || port < 1 || port > 65535 {
			fmt.Println(red + "Error: Invalid port number" + reset)
			os.Exit(1)
		}
		printResult(checkPort(port, showPID), showPID)
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
`, bold, cyan, reset, yellow, reset, yellow, reset, yellow, reset)
}

func checkPort(port int, getPID bool) PortResult {
	result := PortResult{Port: port}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		result.InUse = true
		if getPID {
			result.PID, result.Process = findProcessByPort(port)
		}
	} else {
		listener.Close()
	}
	return result
}

func checkPortRange(start, end int, showPID bool) {
	var wg sync.WaitGroup
	results := make(chan PortResult, end-start+1)
	sem := make(chan struct{}, 100)

	fmt.Printf("%sScanning ports %d-%d...%s\n\n", cyan, start, end, reset)
	startTime := time.Now()

	for port := start; port <= end; port++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results <- checkPort(p, showPID)
		}(port)
	}

	go func() { wg.Wait(); close(results) }()

	var portResults []PortResult
	for r := range results {
		portResults = append(portResults, r)
	}

	// Sort by port
	for i := range portResults {
		for j := i + 1; j < len(portResults); j++ {
			if portResults[i].Port > portResults[j].Port {
				portResults[i], portResults[j] = portResults[j], portResults[i]
			}
		}
	}

	inUse := 0
	for _, r := range portResults {
		if r.InUse {
			inUse++
			printResult(r, showPID)
		}
	}

	fmt.Printf("\n%s%d ports scanned in %v | %d in use, %d available%s\n",
		cyan, len(portResults), time.Since(startTime).Round(time.Millisecond), inUse, len(portResults)-inUse, reset)
}

func printResult(r PortResult, showPID bool) {
	if r.InUse {
		info := fmt.Sprintf("Port %s%d%s is %s%sin use%s", bold, r.Port, reset, red, bold, reset)
		if showPID && r.PID > 0 {
			info += fmt.Sprintf(" (PID: %s%d%s, Process: %s%s%s)", yellow, r.PID, reset, cyan, r.Process, reset)
		} else if showPID {
			info += fmt.Sprintf(" %s(process info unavailable - may need root)%s", yellow, reset)
		}
		fmt.Printf("%s●%s %s\n", red, reset, info)
	} else {
		fmt.Printf("%s○%s Port %s%d%s is %s%savailable%s\n", green, reset, bold, r.Port, reset, green, bold, reset)
	}
}

func findProcessByPort(port int) (int, string) {
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		if pid, name := searchNetFile(f, port); pid > 0 {
			return pid, name
		}
	}
	return 0, ""
}

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
		parts := strings.Split(fields[1], ":")
		if len(parts) == 2 && parts[1] == portHex && fields[3] == "0A" {
			return findPIDByInode(fields[9])
		}
	}
	return 0, ""
}

func findPIDByInode(inode string) (int, string) {
	procDir, err := os.Open("/proc")
	if err != nil {
		return 0, ""
	}
	defer procDir.Close()

	entries, _ := procDir.Readdirnames(-1)
	socketLink := "socket:[" + inode + "]"

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry)
		if err != nil {
			continue
		}
		fdPath := filepath.Join("/proc", entry, "fd")
		fds, err := os.ReadDir(fdPath)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			if link, err := os.Readlink(filepath.Join(fdPath, fd.Name())); err == nil && link == socketLink {
				comm, _ := os.ReadFile(filepath.Join("/proc", entry, "comm"))
				return pid, strings.TrimSpace(string(comm))
			}
		}
	}
	return 0, ""
}

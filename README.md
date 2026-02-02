# portcheck

A fast, simple CLI tool to check if ports are open or in use. Built with Go, no external dependencies.

![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

## Features

- 🔍 **Single port check** — Instantly see if a port is available
- 📊 **Range scanning** — Check multiple ports at once with goroutines
- 🔎 **Process detection** — Find out what's using a port (Linux)
- 🎨 **Colorized output** — Easy-to-read terminal output
- ⚡ **Fast** — Concurrent scanning with goroutines

## Installation

### From source

```bash
git clone https://github.com/kai-wave/portcheck.git
cd portcheck
go build -o portcheck .
```

### Move to PATH (optional)

```bash
sudo mv portcheck /usr/local/bin/
```

## Usage

### Check a single port

```bash
portcheck 8080
```

Output:
```
● Port 8080 is in use
```
or
```
○ Port 8080 is available
```

### Scan a range of ports

```bash
portcheck 3000-3010
```

Output:
```
Scanning ports 3000-3010...

● Port 3000 is in use
● Port 3001 is in use

11 ports scanned in 15ms | 2 in use, 9 available
```

### Find process using a port

```bash
portcheck --pid 22
```

Output:
```
● Port 22 is in use (PID: 1234, Process: sshd)
```

> **Note:** Process detection requires read access to `/proc`. Run with `sudo` if you see "(process info unavailable)".

## Examples

```bash
# Check if your dev server port is free
portcheck 3000

# Scan common web ports
portcheck 80-443

# Find what's hogging port 8080
portcheck --pid 8080

# Quick service check
portcheck 22 && echo "SSH port available" || echo "SSH is running"
```

## How it works

1. **Port checking**: Attempts to bind to the port. If it fails, the port is in use.
2. **Range scanning**: Uses goroutines with a semaphore (100 concurrent) to scan fast without hitting file descriptor limits.
3. **Process detection**: Parses `/proc/net/tcp{,6}` to find socket inodes, then searches `/proc/*/fd/` to match inodes to PIDs.

## Limitations

- Process detection only works on Linux (uses `/proc` filesystem)
- May need root/sudo to detect processes owned by other users
- Port range limited to 1-65535

## License

MIT License - do whatever you want with it!

---

Made with ☕ while learning Go

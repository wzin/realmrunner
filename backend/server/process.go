package server

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type Process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	mu     sync.Mutex
}

func StartProcess(serverDir string, port int, command string, args []string) (*Process, error) {
	// Check if server.jar exists
	jarPath := filepath.Join(serverDir, "server.jar")
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("server.jar not found in %s", serverDir)
	}

	// Accept EULA if not already done
	eulaPath := filepath.Join(serverDir, "eula.txt")
	if err := os.WriteFile(eulaPath, []byte("eula=true\n"), 0644); err != nil {
		return nil, fmt.Errorf("failed to accept EULA: %w", err)
	}

	// Ensure server.properties exists with correct port
	propsPath := filepath.Join(serverDir, "server.properties")
	if _, err := os.Stat(propsPath); os.IsNotExist(err) {
		// Create default server.properties
		props := fmt.Sprintf("server-port=%d\n", port)
		if err := os.WriteFile(propsPath, []byte(props), 0644); err != nil {
			return nil, fmt.Errorf("failed to create server.properties: %w", err)
		}
	}

	// Start process
	cmd := exec.Command(command, args...)
	cmd.Dir = serverDir

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	process := &Process{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}

	// Start log capture to file
	go process.captureOutput(serverDir)

	return process, nil
}

func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd == nil || p.cmd.Process == nil {
		return nil // Already stopped
	}

	// Send "stop" command to Minecraft server
	p.stdin.Write([]byte("stop\n"))

	// Wait for process to exit (the monitorProcess goroutine handles cmd.Wait)
	// We just poll for the process to disappear
	pid := p.cmd.Process.Pid
	for i := 0; i < 300; i++ { // 30 seconds (100ms intervals)
		// Check if process still exists
		if err := p.cmd.Process.Signal(syscall.Signal(0)); err != nil {
			return nil // Process is gone
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill if still running after 30s
	log.Printf("Force killing server process %d after 30s timeout", pid)
	p.cmd.Process.Signal(syscall.SIGKILL)
	time.Sleep(1 * time.Second)

	return nil
}

func (p *Process) ForceKill() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Signal(syscall.SIGKILL)
	}
}

func (p *Process) SendCommand(command string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stdin == nil {
		return fmt.Errorf("stdin not available")
	}

	_, err := p.stdin.Write([]byte(command + "\n"))
	return err
}

func (p *Process) captureOutput(serverDir string) {
	// Ensure logs directory exists
	logsDir := filepath.Join(serverDir, "logs")
	os.MkdirAll(logsDir, 0755)

	// Open log file
	logPath := filepath.Join(logsDir, "latest.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer logFile.Close()

	// Merge stdout and stderr
	merged := io.MultiReader(p.stdout, p.stderr)
	scanner := bufio.NewScanner(merged)

	for scanner.Scan() {
		line := scanner.Text()
		logFile.WriteString(line + "\n")
	}
}

func (p *Process) TailLogs(serverDir string) (<-chan string, error) {
	logPath := filepath.Join(serverDir, "logs", "latest.log")

	ch := make(chan string, 100)

	go func() {
		defer close(ch)

		// Wait for log file to exist
		for i := 0; i < 30; i++ {
			if _, err := os.Stat(logPath); err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		file, err := os.Open(logPath)
		if err != nil {
			log.Printf("Failed to open log file: %v", err)
			return
		}
		defer file.Close()

		// Read from beginning (not end)
		scanner := bufio.NewScanner(file)

		// Send existing lines
		for scanner.Scan() {
			ch <- scanner.Text()
		}

		// Now tail new lines
		for {
			if scanner.Scan() {
				ch <- scanner.Text()
			} else {
				// Check if there's new content
				time.Sleep(100 * time.Millisecond)

				// Re-check file for new content
				newFile, err := os.Open(logPath)
				if err != nil {
					return
				}

				// Seek to where we were
				currentPos, _ := file.Seek(0, io.SeekCurrent)
				newFile.Seek(currentPos, io.SeekStart)

				file.Close()
				file = newFile
				scanner = bufio.NewScanner(file)
			}
		}
	}()

	return ch, nil
}

// PID returns the process ID, or 0 if not running
func (p *Process) PID() int {
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// ReadHistoricalLogs reads logs from the log file (for stopped servers)
// Returns up to the last 10,000 lines to avoid overwhelming the browser
func ReadHistoricalLogs(serverDir string) ([]string, error) {
	logPath := filepath.Join(serverDir, "logs", "latest.log")

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return []string{}, nil // No logs yet
	}

	file, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())

		// Keep only the last 10,000 lines to avoid memory issues
		if len(lines) > 10000 {
			lines = lines[1:]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	return lines, nil
}

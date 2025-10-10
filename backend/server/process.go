package server

import (
	"bufio"
	"fmt"
	"io"
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

func StartProcess(serverDir string, port int, memoryMB int) (*Process, error) {
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

	// Start Java process
	cmd := exec.Command("java",
		fmt.Sprintf("-Xmx%dM", memoryMB),
		fmt.Sprintf("-Xms%dM", memoryMB),
		"-jar", "server.jar",
		"nogui",
	)
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
		return fmt.Errorf("process not running")
	}

	// Send "stop" command to Minecraft server
	p.stdin.Write([]byte("stop\n"))

	// Wait for graceful shutdown (30 seconds)
	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()

	select {
	case <-time.After(30 * time.Second):
		// Force kill if not stopped gracefully
		if err := p.cmd.Process.Signal(syscall.SIGKILL); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		<-done // Wait for process to actually die
	case err := <-done:
		if err != nil && err.Error() != "signal: killed" {
			return fmt.Errorf("process exited with error: %w", err)
		}
	}

	return nil
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

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("log file not found")
	}

	ch := make(chan string, 100)

	go func() {
		defer close(ch)

		file, err := os.Open(logPath)
		if err != nil {
			return
		}
		defer file.Close()

		// Seek to end
		file.Seek(0, io.SeekEnd)

		scanner := bufio.NewScanner(file)
		for {
			if scanner.Scan() {
				ch <- scanner.Text()
			} else {
				// Wait a bit before checking again
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	return ch, nil
}

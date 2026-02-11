package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// capturedOutput captures writes to a buffer while also forwarding to a target writer.
type capturedOutput struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	maxBytes int
}

func (co *capturedOutput) Write(p []byte) (n int, err error) {
	co.mu.Lock()
	defer co.mu.Unlock()
	// Trim from front if buffer exceeds max
	if co.buf.Len()+len(p) > co.maxBytes {
		data := co.buf.Bytes()
		keep := co.maxBytes / 2
		if len(data) > keep {
			data = data[len(data)-keep:]
		}
		co.buf.Reset()
		co.buf.Write(data)
	}
	co.buf.Write(p)
	return len(p), nil
}

func (co *capturedOutput) String() string {
	co.mu.Lock()
	defer co.mu.Unlock()
	return co.buf.String()
}

func (co *capturedOutput) Reset() {
	co.mu.Lock()
	defer co.mu.Unlock()
	co.buf.Reset()
}

// ServiceManager manages services (dev server, etc.)
type ServiceManager struct {
	projectRoot string
	services    []ServiceConfig
	processes   map[string]*exec.Cmd
	outputs     map[string]*capturedOutput
	httpClient  *http.Client
}

// NewServiceManager creates a new service manager
func NewServiceManager(projectRoot string, services []ServiceConfig) *ServiceManager {
	return &ServiceManager{
		projectRoot: projectRoot,
		services:    services,
		processes:   make(map[string]*exec.Cmd),
		outputs:     make(map[string]*capturedOutput),
		httpClient:  &http.Client{Timeout: 2 * time.Second},
	}
}

// EnsureRunning ensures all services are running and ready
func (sm *ServiceManager) EnsureRunning() error {
	for _, svc := range sm.services {
		if err := sm.ensureServiceRunning(svc); err != nil {
			return fmt.Errorf("service %s: %w", svc.Name, err)
		}
	}
	return nil
}

// RestartForVerify restarts services that have restartBeforeVerify=true
func (sm *ServiceManager) RestartForVerify() error {
	for _, svc := range sm.services {
		if svc.RestartBeforeVerify {
			fmt.Printf("Restarting service: %s\n", svc.Name)
			if err := sm.restartService(svc); err != nil {
				return fmt.Errorf("failed to restart %s: %w", svc.Name, err)
			}
		}
	}
	return nil
}

// StopAll stops all managed services. Safe to call multiple times (idempotent).
func (sm *ServiceManager) StopAll() {
	if sm.processes == nil {
		return // Already stopped
	}
	for name, cmd := range sm.processes {
		if cmd.Process != nil {
			fmt.Printf("Stopping service: %s\n", name)
			// Signal the process group so child processes are also terminated
			syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)

			// Wait briefly, then force kill the group
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()

			select {
			case <-done:
				// Process exited
			case <-time.After(5 * time.Second):
				syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		}
	}
	sm.processes = nil // Mark as stopped
}

// ensureServiceRunning ensures a single service is running
func (sm *ServiceManager) ensureServiceRunning(svc ServiceConfig) error {
	// Check if already ready
	if sm.isReady(svc.Ready) {
		return nil
	}

	// Start if we have a start command
	if svc.Start != "" {
		if err := sm.startService(svc); err != nil {
			return err
		}
	}

	// Wait for ready
	return sm.waitForReady(svc)
}

// startService starts a service
func (sm *ServiceManager) startService(svc ServiceConfig) error {
	// Stop if already running
	if cmd, exists := sm.processes[svc.Name]; exists && cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(time.Second)
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		cmd.Wait()
	}

	// Start the service with output capture
	cmd := exec.Command("sh", "-c", svc.Start)
	cmd.Dir = sm.projectRoot
	co := &capturedOutput{maxBytes: 256 * 1024}
	sm.outputs[svc.Name] = co
	cmd.Stdout = co
	cmd.Stderr = co
	
	// Set process group so we can kill all children
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}

	sm.processes[svc.Name] = cmd
	fmt.Printf("Started service: %s (PID %d)\n", svc.Name, cmd.Process.Pid)
	
	return nil
}

// restartService restarts a service
func (sm *ServiceManager) restartService(svc ServiceConfig) error {
	// Stop if running
	if cmd, exists := sm.processes[svc.Name]; exists && cmd.Process != nil {
		// Kill the process group
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(time.Second)
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		cmd.Wait()
		delete(sm.processes, svc.Name)
	}

	// Wait a moment for ports to be released
	time.Sleep(time.Second)

	// Start fresh
	if svc.Start != "" {
		if err := sm.startService(svc); err != nil {
			return err
		}
		return sm.waitForReady(svc)
	}

	return nil
}

// waitForReady waits for a service to be ready
func (sm *ServiceManager) waitForReady(svc ServiceConfig) error {
	timeout := time.Duration(svc.ReadyTimeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for ready (%s)", svc.Ready)
		case <-ticker.C:
			if sm.isReady(svc.Ready) {
				fmt.Printf("Service ready: %s\n", svc.Name)
				return nil
			}
		}
	}
}

// isReady checks if a URL is responding
func (sm *ServiceManager) isReady(url string) bool {
	resp, err := sm.httpClient.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// HasServices returns true if there are services configured
func (sm *ServiceManager) HasServices() bool {
	return len(sm.services) > 0
}

// HasUIServices returns true if there are services with restartBeforeVerify
func (sm *ServiceManager) HasUIServices() bool {
	for _, svc := range sm.services {
		if svc.RestartBeforeVerify {
			return true
		}
	}
	return false
}

// CheckServiceHealth checks if all started services are still responding.
func (sm *ServiceManager) CheckServiceHealth() []string {
	var issues []string
	for _, svc := range sm.services {
		if _, started := sm.processes[svc.Name]; started {
			if !sm.isReady(svc.Ready) {
				issues = append(issues, fmt.Sprintf("service '%s' not responding at %s", svc.Name, svc.Ready))
			}
		}
	}
	return issues
}

// GetRecentOutput returns the last maxLines lines of captured output for a service.
func (sm *ServiceManager) GetRecentOutput(name string, maxLines int) string {
	co, ok := sm.outputs[name]
	if !ok {
		return ""
	}
	output := co.String()
	lines := strings.Split(output, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}


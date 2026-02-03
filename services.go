package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// ServiceManager manages services (dev server, etc.)
type ServiceManager struct {
	projectRoot string
	services    []ServiceConfig
	processes   map[string]*exec.Cmd
}

// NewServiceManager creates a new service manager
func NewServiceManager(projectRoot string, services []ServiceConfig) *ServiceManager {
	return &ServiceManager{
		projectRoot: projectRoot,
		services:    services,
		processes:   make(map[string]*exec.Cmd),
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

// StopAll stops all managed services
func (sm *ServiceManager) StopAll() {
	for name, cmd := range sm.processes {
		if cmd.Process != nil {
			fmt.Printf("Stopping service: %s\n", name)
			// Try graceful shutdown first
			cmd.Process.Signal(syscall.SIGTERM)
			
			// Wait briefly, then force kill
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			
			select {
			case <-done:
				// Process exited
			case <-time.After(5 * time.Second):
				// Force kill
				cmd.Process.Kill()
			}
		}
	}
	sm.processes = make(map[string]*exec.Cmd)
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
		cmd.Process.Kill()
		cmd.Wait()
	}

	// Start the service
	cmd := exec.Command("sh", "-c", svc.Start)
	cmd.Dir = sm.projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
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
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
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

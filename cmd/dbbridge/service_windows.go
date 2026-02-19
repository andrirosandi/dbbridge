package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "DbBridge"
const serviceDisplayName = "DbBridge Database API Server"
const serviceDescription = "DbBridge - Database Bridge API Server for executing predefined SQL queries"

// dbBridgeService implements the svc.Handler interface
type dbBridgeService struct {
	stopCh chan struct{}
}

// Execute is called by the Windows Service Control Manager
func (s *dbBridgeService) Execute(args []string, changeReq <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	status <- svc.Status{State: svc.StartPending}

	// Change to executable directory so templates/static files are found
	exePath, err := os.Executable()
	if err == nil {
		os.Chdir(filepath.Dir(exePath))
	}

	// Start the server in a goroutine
	s.stopCh = make(chan struct{})
	go func() {
		startServer()
	}()

	status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// Wait for stop/shutdown signal
	for {
		c := <-changeReq
		switch c.Cmd {
		case svc.Interrogate:
			status <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending}
			close(s.stopCh)
			// Give server time to gracefully shutdown
			time.Sleep(2 * time.Second)
			return false, 0
		}
	}
}

// isRunningAsService checks if the process is running as a Windows Service
func isRunningAsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}

// runAsService starts the application as a Windows Service
func runAsService() {
	err := svc.Run(serviceName, &dbBridgeService{})
	if err != nil {
		fmt.Printf("Failed to run as service: %v\n", err)
		os.Exit(1)
	}
}

// installService registers DbBridge as a Windows Service
func installService() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("Failed to get executable path: %v\n", err)
		os.Exit(1)
	}

	m, err := mgr.Connect()
	if err != nil {
		fmt.Printf("Failed to connect to service manager: %v\n", err)
		fmt.Println("Hint: Run this command as Administrator.")
		os.Exit(1)
	}
	defer m.Disconnect()

	// Check if service already exists
	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		fmt.Printf("Service '%s' is already installed.\n", serviceName)
		return
	}

	s, err = m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
		StartType:   mgr.StartAutomatic,
	})
	if err != nil {
		fmt.Printf("Failed to install service: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	fmt.Printf("Service '%s' installed successfully.\n", serviceName)
	fmt.Println("Start with: dbbridge start")
	fmt.Println("Or via: services.msc")
}

// uninstallService removes DbBridge from Windows Services
func uninstallService() {
	m, err := mgr.Connect()
	if err != nil {
		fmt.Printf("Failed to connect to service manager: %v\n", err)
		fmt.Println("Hint: Run this command as Administrator.")
		os.Exit(1)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		fmt.Printf("Service '%s' is not installed.\n", serviceName)
		return
	}
	defer s.Close()

	err = s.Delete()
	if err != nil {
		fmt.Printf("Failed to uninstall service: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Service '%s' uninstalled successfully.\n", serviceName)
}

// startService starts the DbBridge Windows Service
func startService() {
	m, err := mgr.Connect()
	if err != nil {
		fmt.Printf("Failed to connect to service manager: %v\n", err)
		fmt.Println("Hint: Run this command as Administrator.")
		os.Exit(1)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		fmt.Printf("Service '%s' is not installed. Run 'dbbridge install' first.\n", serviceName)
		os.Exit(1)
	}
	defer s.Close()

	err = s.Start()
	if err != nil {
		fmt.Printf("Failed to start service: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Service '%s' started.\n", serviceName)
}

// stopService stops the DbBridge Windows Service
func stopService() {
	cmd := exec.Command("sc", "stop", serviceName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Failed to stop service: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Service '%s' stopped.\n", serviceName)
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/natefinch/lumberjack"
	"github.com/sirupsen/logrus"
)

type Config struct {
	RecycleBinDir    string `json:"recycleBinDir"`
	NumWorkers       int    `json:"numWorkers"`
	CheckIntervalSec int    `json:"checkIntervalSec"`
	MaxRetries       int    `json:"maxRetries"`
	RetryDelaySec    int    `json:"retryDelaySec"`
}

type HealthStatus struct {
	Timestamp          time.Time `json:"timestamp"`
	ProgramRunning     bool      `json:"program_running"`
	RecycleBinExists   bool      `json:"recycle_bin_exists"`
	RecycleFileExists  bool      `json:"recycle_file_exists"`
	AliasExists        bool      `json:"alias_exists"`
	NFSExists          bool      `json:"nfs_exists"`
	NFSCheckInProgress bool      `json:"nfs_check_in_progress"`
	LastError          string    `json:"last_error,omitempty"`
	LastNFSCheck       time.Time `json:"last_nfs_check"`
	NFSCheckAttempts   int       `json:"nfs_check_attempts"`
	NFSCheckMaxRetries int       `json:"nfs_check_max_retries"`
	Health             string    `json:"health"` // New field to indicate health status
}

type HealthChecker struct {
	config         Config
	status         HealthStatus
	statusMutex    sync.RWMutex
	logger         *logrus.Logger
	stopChan       chan struct{}
	httpServer     *http.Server
	nfsCheckTicker *time.Ticker
}

const (
	healthCheckPort = ":10001"
	nfsCheckTimeout = 5 * time.Second
)

func main() {
	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetLevel(logrus.DebugLevel)

	// Create log file with rotation
	logFile := &lumberjack.Logger{
		Filename:   "/var/log/cbin/health-checker.log",
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
		Compress:   true,
	}
	logger.SetOutput(logFile)

	// Load configuration
	config, err := loadConfig("/etc/cbin/config.conf")
	if err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}

	// Initialize health checker
	checker := NewHealthChecker(config, logger)

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start health checker
	go checker.Start()

	// Start HTTP server
	go checker.startHTTPServer()

	// Wait for shutdown signal
	<-sigChan
	checker.Stop()
}

func NewHealthChecker(config Config, logger *logrus.Logger) *HealthChecker {
	return &HealthChecker{
		config:   config,
		logger:   logger,
		stopChan: make(chan struct{}),
		status: HealthStatus{
			Timestamp:          time.Now(),
			LastNFSCheck:       time.Now(),
			NFSCheckMaxRetries: config.MaxRetries,
			Health:             "fail", // Default to fail
		},
	}
}

func (hc *HealthChecker) Start() {
	// Start separate goroutine for NFS checks
	go hc.startNFSChecker()

	// Start regular health checks
	ticker := time.NewTicker(time.Duration(hc.config.CheckIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hc.performBasicHealthCheck()
		case <-hc.stopChan:
			return
		}
	}
}

func (hc *HealthChecker) Stop() {
	close(hc.stopChan)

	// Shutdown the HTTP server gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := hc.httpServer.Shutdown(ctx); err != nil {
		hc.logger.Errorf("Error shutting down HTTP server: %v", err)
	}
}

func (hc *HealthChecker) startNFSChecker() {
	ticker := time.NewTicker(time.Duration(hc.config.CheckIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hc.performNFSCheck()
		case <-hc.stopChan:
			return
		}
	}
}

func (hc *HealthChecker) performBasicHealthCheck() {
	hc.statusMutex.Lock()
	defer hc.statusMutex.Unlock()

	hc.status.Timestamp = time.Now()

	// Perform quick checks
	hc.status.ProgramRunning = hc.checkProcessRunning()
	hc.status.RecycleBinExists = hc.checkRecycleBinExists()
	hc.status.RecycleFileExists = hc.checkRecycleFileExists()
	hc.status.AliasExists = hc.checkAliasExists()

	// Determine overall health status
	hc.status.Health = "pass"
	if !hc.status.ProgramRunning || !hc.status.NFSExists || !hc.status.RecycleBinExists || !hc.status.RecycleFileExists {
		hc.status.Health = "fail"
	}

	// Check if critical conditions are met
	if !hc.status.ProgramRunning || !hc.status.NFSExists {
		hc.logger.Warn("Critical health check failed, removing rm alias")
		if err := hc.removeAlias(); err != nil {
			hc.logger.Errorf("Failed to remove alias: %v", err)
		}
	}

	// Check if all conditions are met and alias doesn't exist
	if hc.status.ProgramRunning && hc.status.NFSExists && hc.status.RecycleBinExists && hc.status.RecycleFileExists && !hc.status.AliasExists {
		hc.logger.Info("All conditions met, restoring rm alias")
		if err := hc.addAlias(); err != nil {
			hc.logger.Errorf("Failed to add alias: %v", err)
		} else {
			hc.status.AliasExists = true
		}
	}

	hc.logStatus()
}

func (hc *HealthChecker) performNFSCheck() {
	hc.statusMutex.Lock()
	hc.status.NFSCheckInProgress = true
	hc.status.NFSCheckAttempts = 0
	hc.statusMutex.Unlock()

	// Create context with timeout for NFS check
	ctx, cancel := context.WithTimeout(context.Background(), nfsCheckTimeout)
	defer cancel()

	success := false
	for i := 0; i < hc.config.MaxRetries; i++ {
		hc.statusMutex.Lock()
		hc.status.NFSCheckAttempts = i + 1
		hc.statusMutex.Unlock()

		if result := hc.checkNFSMount(); result {
			success = true
			break
		} else {
			// Update status immediately after each failed attempt
			hc.statusMutex.Lock()
			hc.status.NFSExists = false
			hc.status.LastError = fmt.Sprintf("NFS mount check failed (attempt %d/%d)",
				i+1, hc.config.MaxRetries)
			hc.status.LastNFSCheck = time.Now()
			hc.statusMutex.Unlock()

			hc.logger.Warnf("NFS check failed, attempt %d/%d", i+1, hc.config.MaxRetries)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(hc.config.RetryDelaySec) * time.Second):
			continue
		}
	}

	hc.statusMutex.Lock()
	hc.status.NFSExists = success
	hc.status.NFSCheckInProgress = false
	hc.status.LastNFSCheck = time.Now()
	if !success {
		hc.status.LastError = fmt.Sprintf("NFS mount check failed after %d attempts",
			hc.config.MaxRetries)
	} else {
		hc.status.LastError = ""
	}
	hc.statusMutex.Unlock()

	// Update overall health status based on NFS check result
	hc.statusMutex.Lock()
	defer hc.statusMutex.Unlock()
	if !success {
		hc.status.Health = "fail"
	} else {
		hc.status.Health = "pass"
	}
}

func (hc *HealthChecker) checkNFSMount() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "df", "-t", "nfs", "-t", "nfs4")
	out, err := cmd.Output()
	if err != nil {
		hc.logger.Errorf("Failed to check NFS mount: %v", err)
		return false
	}
	return len(out) > 0
}

func (hc *HealthChecker) checkProcessRunning() bool {
	cmd := exec.Command("pgrep", "-f", "cbin")
	if err := cmd.Run(); err != nil {
		hc.status.LastError = fmt.Sprintf("cbin process not running: %v", err)
		return false
	}
	return true
}

func (hc *HealthChecker) checkRecycleBinExists() bool {
	if _, err := os.Stat(hc.config.RecycleBinDir); err != nil {
		hc.status.LastError = fmt.Sprintf("recycle bin directory not accessible: %v", err)
		return false
	}
	return true
}

func (hc *HealthChecker) checkRecycleFileExists() bool {
	if _, err := os.Stat("/usr/local/bin/cbin"); err != nil {
		hc.status.LastError = fmt.Sprintf("cbin executable not found: %v", err)
		return false
	}
	return true
}

func (hc *HealthChecker) checkAliasExists() bool {
	content, err := ioutil.ReadFile("/etc/bash.bashrc")
	if err != nil {
		hc.status.LastError = fmt.Sprintf("failed to read bash.bashrc: %v", err)
		hc.logger.Errorf("Failed to read bash.bashrc: %v", err)
		return false
	}

	aliasLine := `alias rm='/usr/local/bin/cbin'`
	return strings.Contains(string(content), aliasLine)
}

func (hc *HealthChecker) removeAlias() error {
	content, err := ioutil.ReadFile("/etc/bash.bashrc")
	if err != nil {
		return fmt.Errorf("failed to read bash.bashrc: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	for _, line := range lines {
		if !strings.Contains(line, `alias rm='/usr/local/bin/cbin'`) {
			newLines = append(newLines, line)
		}
	}

	err = ioutil.WriteFile("/etc/bash.bashrc", []byte(strings.Join(newLines, "\n")), 0644)
	if err != nil {
		return fmt.Errorf("failed to write bash.bashrc: %v", err)
	}

	cmd := exec.Command("bash", "-c", "source /etc/bash.bashrc")
	return cmd.Run()
}

func (hc *HealthChecker) addAlias() error {
	content, err := ioutil.ReadFile("/etc/bash.bashrc")
	if err != nil {
		return fmt.Errorf("failed to read bash.bashrc: %v", err)
	}

	aliasLine := `alias rm='/usr/local/bin/cbin'`
	if strings.Contains(string(content), aliasLine) {
		return nil // Alias already exists
	}

	newContent := string(content) + "\n" + aliasLine
	err = ioutil.WriteFile("/etc/bash.bashrc", []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write bash.bashrc: %v", err)
	}

	cmd := exec.Command("bash", "-c", "source /etc/bash.bashrc")
	return cmd.Run()
}

func (hc *HealthChecker) startHTTPServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", hc.healthHandler)

	hc.httpServer = &http.Server{
		Addr:         healthCheckPort,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	hc.logger.Infof("Starting health check server on port %s", healthCheckPort)

	if err := hc.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		hc.logger.Fatalf("Failed to start HTTP server: %v", err)
	}
}

func (hc *HealthChecker) healthHandler(w http.ResponseWriter, r *http.Request) {
	hc.statusMutex.RLock()
	status := hc.status // Create a copy of the status
	hc.statusMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, fmt.Sprintf("Error encoding response: %v", err),
			http.StatusInternalServerError)
		return
	}
}

func (hc *HealthChecker) logStatus() {
	hc.logger.WithFields(logrus.Fields{
		"status":             hc.status,
		"program_running":    hc.status.ProgramRunning,
		"nfs_exists":         hc.status.NFSExists,
		"alias_exists":       hc.status.AliasExists,
		"timestamp":          hc.status.Timestamp,
		"nfs_check_attempts": hc.status.NFSCheckAttempts,
		"health":             hc.status.Health, // Log the health status
	}).Info("Health check completed")
}

func loadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return Config{}, fmt.Errorf("failed to decode config: %v", err)
	}

	// Set default values if not specified
	if config.CheckIntervalSec == 0 {
		config.CheckIntervalSec = 60 // Default to 1 minute
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 5 // Default to 5 retries
	}
	if config.RetryDelaySec == 0 {
		config.RetryDelaySec = 60 // Default to 1 minute between retries
	}

	return config, nil
}

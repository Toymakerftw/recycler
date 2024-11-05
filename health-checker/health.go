package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	clientIP       string
	masterIP       string
	serverPort     = ":10002"
	retryCount     = 5
	retryDelay     = 1 * time.Minute
	recycleBinDir  string
	configFilePath = "/etc/recycler-cli/config.conf"
	envFilePath    = "/etc/recycler-cli/env"
)

type HealthResponse struct {
	ProgramRunning      bool   `json:"program_running"`
	RecycleBinExists    bool   `json:"recycle_bin_exists"`
	LogDirExists        bool   `json:"log_dir_exists"`
	ConfigFileExists    bool   `json:"config_file_exists"`
	AliasExists         bool   `json:"alias_exists"`
	NFSExists           bool   `json:"nfs_exists"`
	OverallHealthStatus string `json:"overall_health_status"`
}

func main() {
	// Load environment and configuration files
	loadEnv(envFilePath)
	loadConfig(configFilePath)

	// Start the HTTP server
	startHTTPServer()
}

// Load environment variables from a file
func loadEnv(envFile string) {
	file, err := os.Open(envFile)
	if err != nil {
		fmt.Println("Error opening env file:", err)
		os.Exit(1)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "client_ip":
			clientIP = value
		case "master_ip":
			masterIP = value
		}
	}
}

// Load the configuration file and set the recycle bin directory
func loadConfig(configFile string) {
	file, err := os.Open(configFile)
	if err != nil {
		fmt.Println("Error opening config file:", err)
		os.Exit(1)
	}
	defer file.Close()

	var config struct {
		RecycleBinDir string `json:"recycleBinDir"`
	}
	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		fmt.Println("Error decoding config file:", err)
		os.Exit(1)
	}
	recycleBinDir = config.RecycleBinDir
}

// Start the HTTP server for health checks
func startHTTPServer() {
	http.HandleFunc("/health", healthHandler)
	fmt.Println("Starting health-check server on", clientIP+serverPort)
	err := http.ListenAndServe(clientIP+serverPort, nil)
	if err != nil {
		fmt.Println("Error starting HTTP server:", err)
	}
}

// HTTP handler for health status
func healthHandler(w http.ResponseWriter, r *http.Request) {
	healthResponse := evaluateHealth()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(healthResponse)
}

// Check the overall health status
func evaluateHealth() HealthResponse {
	healthResponse := HealthResponse{
		ProgramRunning:   checkProgramRunning(),
		RecycleBinExists: checkRecycleBin(),
		LogDirExists:     checkLogDir(),
		ConfigFileExists: checkConfigFile(),
		AliasExists:      checkAlias(),
		NFSExists:        checkNFSWithRetries(),
	}

	if !healthResponse.ProgramRunning || !healthResponse.RecycleBinExists || !healthResponse.LogDirExists ||
		!healthResponse.ConfigFileExists || !healthResponse.AliasExists || !healthResponse.NFSExists {
		healthResponse.OverallHealthStatus = "failed"
	} else {
		healthResponse.OverallHealthStatus = "ok"
	}

	return healthResponse
}

// Check if the main recycler-cli program is running
func checkProgramRunning() bool {
	out, err := exec.Command("pgrep", "-f", "recycler-cli").Output()
	if err != nil || len(out) == 0 {
		return false
	}
	return true
}

// Check if the recycle bin directory exists
func checkRecycleBin() bool {
	if recycleBinDir == "" {
		fmt.Println("Recycle bin directory not set in config.")
		return false
	}
	if _, err := os.Stat(recycleBinDir); err == nil {
		return true
	}
	return false
}

// Check if the log directory exists
func checkLogDir() bool {
	logDir := "/var/log/recycler-cli"
	if _, err := os.Stat(logDir); err == nil {
		return true
	}
	return false
}

// Check if the configuration file exists
func checkConfigFile() bool {
	if _, err := os.Stat(configFilePath); err == nil {
		return true
	}
	return false
}

// Check if the 'rm' alias points to recycler-cli
func checkAlias() bool {
	file, err := os.Open("/etc/bash.bashrc")
	if err != nil {
		fmt.Println("Error opening bash.bashrc:", err)
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "alias rm='/usr/local/bin/recycler-cli'") {
			return true
		}
	}
	return false
}

// Check if the NFS mount exists with retries
func checkNFSWithRetries() bool {
	for i := 0; i < retryCount; i++ {
		if checkNFS() {
			return true
		}
		fmt.Printf("NFS check failed. Retrying in %v... (Attempt %d/%d)\n", retryDelay, i+1, retryCount)
		time.Sleep(retryDelay)
	}

	// If all retries fail, remove alias and reload bashrc
	removeAliasAndReload()
	return false
}

// Check if NFS mount is available
func checkNFS() bool {
	out, err := exec.Command("df", "-h").Output()
	if err != nil {
		fmt.Println("Error running df command:", err)
		return false
	}

	expectedMount := masterIP + ":/mnt/recycler"
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, expectedMount) {
			return true
		}
	}
	return false
}

// Remove the alias and reload bashrc if NFS fails
func removeAliasAndReload() {
	input, err := os.ReadFile("/etc/bash.bashrc")
	if err != nil {
		fmt.Println("Error reading bash.bashrc:", err)
		return
	}

	output := ""
	for _, line := range strings.Split(string(input), "\n") {
		if !strings.Contains(line, "alias rm='/usr/local/bin/recycler-cli'") {
			output += line + "\n"
		}
	}

	err = os.WriteFile("/etc/bash.bashrc", []byte(output), 0644)
	if err != nil {
		fmt.Println("Error writing to bash.bashrc:", err)
		return
	}

	exec.Command("bash", "-c", "source /etc/bash.bashrc").Run()
}

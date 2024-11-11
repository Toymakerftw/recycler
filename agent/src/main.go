package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

func main() {
	// Define flags
	var (
		help        = flag.Bool("h", false, "Show help")
		forceRemove = flag.Bool("rf", false, "Force remove files or directories (use with caution)")
		files       = flag.String("files", "", "Comma-separated list of files to recycle")
		restore     = flag.Bool("restore", false, "Restore files from recycle bin")
		restoreDate = flag.String("d", "", "Date to restore files from (format: YYYY-MM-DD)")
		singleFile  = flag.String("s", "", "Specify a single file to restore from recycle bin")
	)
	flag.Parse()

	if *help {
		printHelp()
		return
	}

	// Read configuration from file
	config, err := readConfig("/etc/cbin/config.conf")
	if err != nil {
		logrus.Fatalf("Failed to read configuration file: %v", err)
	}

	// Initialize logger
	initLogger()

	// Get server information
	ip, hostname, err := getServerInfo()
	if err != nil {
		logrus.Fatalf("Failed to get server information: %v", err)
	}

	// Create unique server directory in recycle bin
	serverDir := filepath.Join(config.RecycleBinDir, fmt.Sprintf("%s_%s", ip, hostname))

	// Handle positional arguments (files to recycle) separately
	if len(flag.Args()) > 0 {
		// Use positional arguments as the list of files to recycle
		*files = strings.Join(flag.Args(), ",")
	}

	// Process restore or recycle actions
	if *restore {
		restoreFile(serverDir, *restoreDate, *singleFile)
	} else if *files != "" {
		fileSlice := strings.Split(*files, ",")
		recycleFiles(fileSlice, serverDir, config.NumWorkers, ip, hostname, *forceRemove)
	} else {
		logrus.Info("Please specify files to recycle using -rf or as positional arguments.")
		printHelp()
	}
}

func printHelp() {
	fmt.Println("cbin - A centralized recycle bin for Linux servers.")
	fmt.Println("--------------------------------------------------")
	fmt.Println("Usage:")
	fmt.Println(" cbin [options] [files...]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -rf, --force-remove   Force remove files or directories (use with caution)")
	fmt.Println("  -f, --files        Comma-separated list of files to recycle (e.g., file1.txt,file2.log)")
	fmt.Println("  -restore, --restore Restore files from recycle bin")
	fmt.Println("  -d, --date          Date to restore files from (format: YYYY-MM-DD)")
	fmt.Println("  -s, --single-file   Specify a single file to restore from the recycle bin on a given date")
	fmt.Println("  -h, --help          Display this help message")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  cbin -rf file1.txt,file2.log,file3.pdf")
	fmt.Println("  cbin -restore -d 2024-11-02")
	fmt.Println("  cbin -restore -d 2024-11-02 -s file1.txt")
	fmt.Println("  cbin file1.txt")
	fmt.Println()
	fmt.Println("Important:")
	fmt.Println("  - Ensure the recycle bin directory is set to a valid path.")
	fmt.Println("  - Be cautious with file paths to avoid unintended actions.")
}

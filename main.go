package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/natefinch/lumberjack"
	"github.com/sirupsen/logrus"
)

type Config struct {
	RecycleBinDir string `json:"recycleBinDir"`
	NumWorkers    int    `json:"numWorkers"`
}

type FileMetadata struct {
	OriginalPath string    `json:"original_path"`
	OriginalName string    `json:"original_name"`
	CurrentName  string    `json:"current_name"`
	FileSize     int64     `json:"file_size"`
	DeletedAt    time.Time `json:"deleted_at"`
	FileType     string    `json:"file_type"`
}

func main() {
	// Define flags
	var (
		help        = flag.Bool("h", false, "Show help")
		files       = flag.String("f", "", "Comma-separated list of files to recycle (e.g., file1.txt,file2.log)")
		restore     = flag.Bool("r", false, "Restore files from recycle bin")
		restoreDate = flag.String("d", "", "Date to restore files from (format: YYYY-MM-DD)")
		singleFile  = flag.String("s", "", "Specify a single file to restore from recycle bin")
	)
	flag.Parse()

	if *help {
		printHelp()
		return
	}

	// Read configuration from file
	config, err := readConfig("/etc/recycler-cli/config.conf")
	if err != nil {
		logrus.Fatalf("Failed to read configuration file: %v", err)
	}

	// Set up logging with log rotation
	logrus.SetOutput(&lumberjack.Logger{
		Filename:   "/var/log/recycler-cli/recycler-cli.log",
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
	})
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.InfoLevel)

	// Get server's private IP and hostname
	ip, hostname, err := getServerInfo()
	if err != nil {
		logrus.Fatalf("Failed to get server information: %v", err)
	}

	// Create unique directory for the server
	serverDir := filepath.Join(config.RecycleBinDir, fmt.Sprintf("%s_%s", ip, hostname))
	err = os.MkdirAll(serverDir, os.ModePerm)
	if err != nil {
		logrus.Fatalf("Failed to create server directory: %v", err)
	}

	if *restore {
		restoreFile(serverDir, *restoreDate, *singleFile)
	} else {
		// Check if files flag is provided
		if *files == "" {
			logrus.Info("Please specify files to recycle using -f.")
			printHelp()
			return
		}

		// Split comma-separated files into a slice
		fileSlice := strings.Split(*files, ",")

		// Create a channel to send file paths to workers
		fileChan := make(chan string, len(fileSlice))
		for _, file := range fileSlice {
			fileChan <- strings.TrimSpace(file)
		}
		close(fileChan)

		// Create a wait group to wait for all workers to finish
		var wg sync.WaitGroup

		// Start worker goroutines
		for i := 0; i < config.NumWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for file := range fileChan {
					moveFileToRecycleBin(file, serverDir, ip, hostname)
				}
			}()
		}

		// Wait for all workers to finish
		wg.Wait()
		logrus.Info("All specified files have been recycled.")
	}
}

func moveFileToRecycleBin(file string, serverDir string, ip string, hostname string) {
	// Check if file exists
	fileInfo, err := os.Stat(file)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.WithFields(logrus.Fields{
				"file":   file,
				"server": fmt.Sprintf("%s_%s", ip, hostname),
			}).Warn("Skipping non-existent file")
		} else {
			logrus.WithFields(logrus.Fields{
				"file":   file,
				"server": fmt.Sprintf("%s_%s", ip, hostname),
				"error":  err,
			}).Error("Error checking existence of file")
		}
		return
	}

	// Determine the file type before moving the file
	mimeType, err := mimetype.DetectFile(file)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"file":   file,
			"server": fmt.Sprintf("%s_%s", ip, hostname),
			"error":  err,
		}).Error("Failed to detect file type")
		return
	}

	// Get current date and time
	now := time.Now()
	dateDir := filepath.Join(serverDir, now.Format("2006-01-02"))
	err = os.MkdirAll(dateDir, os.ModePerm)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"dateDir": dateDir,
			"server":  fmt.Sprintf("%s_%s", ip, hostname),
			"error":   err,
		}).Error("Failed to create date directory")
		return
	}

	// Construct destination path with timestamp
	timestamp := now.Format("15:04:05")
	destPath := filepath.Join(dateDir, fmt.Sprintf("%s_%s%s", fileInfo.Name(), timestamp, filepath.Ext(file)))

	// Try to move file to recycle bin
	logrus.WithFields(logrus.Fields{
		"file":     file,
		"destPath": destPath,
		"server":   fmt.Sprintf("%s_%s", ip, hostname),
	}).Info("Moving file to recycle bin")
	err = os.Rename(file, destPath)

	// If os.Rename fails with cross-device error, copy and delete the file
	if err != nil {
		if linkErr, ok := err.(*os.LinkError); ok && linkErr.Err.Error() == "invalid cross-device link" {
			logrus.WithFields(logrus.Fields{
				"file":     file,
				"destPath": destPath,
				"server":   fmt.Sprintf("%s_%s", ip, hostname),
			}).Info("Cross-device link error detected. Copying file instead.")

			// Open source file for reading
			srcFile, err := os.Open(file)
			if err != nil {
				logrus.WithFields(logrus.Fields{"file": file, "error": err}).Error("Failed to open source file")
				return
			}
			defer srcFile.Close()

			// Create destination file
			destFile, err := os.Create(destPath)
			if err != nil {
				logrus.WithFields(logrus.Fields{"destPath": destPath, "error": err}).Error("Failed to create destination file")
				return
			}
			defer destFile.Close()

			// Copy contents
			_, err = io.Copy(destFile, srcFile)
			if err != nil {
				logrus.WithFields(logrus.Fields{"file": file, "destPath": destPath, "error": err}).Error("Failed to copy file")
				return
			}

			// Close files before deleting source file
			srcFile.Close()
			destFile.Close()

			// Delete original file
			err = os.Remove(file)
			if err != nil {
				logrus.WithFields(logrus.Fields{"file": file, "error": err}).Error("Failed to delete original file after copying")
				return
			}
			logrus.WithFields(logrus.Fields{
				"file":     file,
				"destPath": destPath,
				"server":   fmt.Sprintf("%s_%s", ip, hostname),
			}).Info("File successfully copied to recycle bin and original deleted")
		} else {
			logrus.WithFields(logrus.Fields{
				"file":     file,
				"destPath": destPath,
				"server":   fmt.Sprintf("%s_%s", ip, hostname),
				"error":    err,
			}).Error("Failed to move file to recycle bin")
			return
		}
	}

	// Write metadata to JSON file
	metadata := FileMetadata{
		OriginalPath: file,
		OriginalName: fileInfo.Name(),
		CurrentName:  filepath.Base(destPath), // Set the CurrentName field
		FileSize:     fileInfo.Size(),
		DeletedAt:    now,
		FileType:     mimeType.String(),
	}
	err = writeMetadata(dateDir, metadata)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"file":   file,
			"server": fmt.Sprintf("%s_%s", ip, hostname),
			"error":  err,
		}).Error("Failed to write metadata")
	}
}

func restoreFile(serverDir string, restoreDate string, singleFile string) {
	var targetDirs []string

	if restoreDate == "" {
		// Restore from all directories (i.e., all dates)
		err := filepath.Walk(serverDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && path != serverDir {
				targetDirs = append(targetDirs, path)
			}
			return nil
		})
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Error("Failed to walk through recycle bin directories")
			return
		}
	} else {
		// Restore from a specific date directory
		targetDirs = append(targetDirs, filepath.Join(serverDir, restoreDate))
	}

	for _, dateDir := range targetDirs {
		metadataFile := filepath.Join(dateDir, "metadata.json")
		if _, err := os.Stat(metadataFile); os.IsNotExist(err) {
			logrus.WithFields(logrus.Fields{
				"metadataFile": metadataFile,
			}).Warn("Metadata file not found, skipping directory")
			continue
		}

		data, err := ioutil.ReadFile(metadataFile)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"metadataFile": metadataFile,
				"error":        err,
			}).Error("Failed to read metadata file")
			continue
		}

		var metadata []FileMetadata
		err = json.Unmarshal(data, &metadata)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"metadataFile": metadataFile,
				"error":        err,
			}).Error("Failed to unmarshal metadata")
			continue
		}

		for _, meta := range metadata {
			// Check if restoring a specific file
			if singleFile != "" && meta.OriginalName != singleFile {
				continue // Skip files that don't match the specified single file
			}

			originalPath := meta.OriginalPath
			currentPath := filepath.Join(dateDir, meta.CurrentName)

			// Check if the file to be restored still exists in the recycle bin
			if _, err := os.Stat(currentPath); os.IsNotExist(err) {
				logrus.WithFields(logrus.Fields{
					"originalPath": originalPath,
					"currentPath":  currentPath,
				}).Warn("File to be restored not found in recycle bin, skipping")
				continue
			}

			// Attempt to restore the file
			logrus.WithFields(logrus.Fields{
				"originalPath": originalPath,
				"currentPath":  currentPath,
			}).Info("Restoring file")
			err = os.Rename(currentPath, originalPath)

			// If os.Rename fails with cross-device error, use copy-and-delete approach
			if err != nil {
				if linkErr, ok := err.(*os.LinkError); ok && linkErr.Err.Error() == "invalid cross-device link" {
					logrus.WithFields(logrus.Fields{
						"currentPath":  currentPath,
						"originalPath": originalPath,
					}).Info("Cross-device link error detected. Copying file instead.")

					// Open source file for reading
					srcFile, err := os.Open(currentPath)
					if err != nil {
						logrus.WithFields(logrus.Fields{
							"currentPath": currentPath,
							"error":       err,
						}).Error("Failed to open source file for copying")
						continue
					}
					defer srcFile.Close()

					// Create destination file
					destFile, err := os.Create(originalPath)
					if err != nil {
						logrus.WithFields(logrus.Fields{
							"originalPath": originalPath,
							"error":        err,
						}).Error("Failed to create destination file")
						continue
					}
					defer destFile.Close()

					// Copy contents
					_, err = io.Copy(destFile, srcFile)
					if err != nil {
						logrus.WithFields(logrus.Fields{
							"currentPath":  currentPath,
							"originalPath": originalPath,
							"error":        err,
						}).Error("Failed to copy file contents")
						continue
					}

					// Close files before deleting source file
					srcFile.Close()
					destFile.Close()

					// Delete original file in recycle bin
					err = os.Remove(currentPath)
					if err != nil {
						logrus.WithFields(logrus.Fields{
							"currentPath": currentPath,
							"error":       err,
						}).Error("Failed to delete file from recycle bin after copying")
						continue
					}
					logrus.WithFields(logrus.Fields{
						"originalPath": originalPath,
						"currentPath":  currentPath,
					}).Info("File successfully restored by copying and original deleted")
				} else {
					logrus.WithFields(logrus.Fields{
						"originalPath": originalPath,
						"currentPath":  currentPath,
						"error":        err,
					}).Error("Failed to restore file")
					continue
				}
			} else {
				logrus.WithFields(logrus.Fields{
					"originalPath": originalPath,
					"currentPath":  currentPath,
				}).Info("File successfully restored")
			}

			// Update metadata after successful restore
			updateMetadata(dateDir, meta, true)

			// Exit after restoring a single file
			if singleFile != "" {
				return
			}
		}
	}
}

func updateMetadata(dateDir string, metadataToRemove FileMetadata, remove bool) {
	metadataFile := filepath.Join(dateDir, "metadata.json")
	data, err := ioutil.ReadFile(metadataFile)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"metadataFile": metadataFile,
			"error":        err,
		}).Error("Failed to read metadata file for update")
		return
	}

	var metadata []FileMetadata
	err = json.Unmarshal(data, &metadata)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"metadataFile": metadataFile,
			"error":        err,
		}).Error("Failed to unmarshal metadata for update")
		return
	}

	var updatedMetadata []FileMetadata
	for _, meta := range metadata {
		if (meta.OriginalPath != metadataToRemove.OriginalPath) || (meta.DeletedAt != metadataToRemove.DeletedAt) {
			updatedMetadata = append(updatedMetadata, meta)
		} else if !remove {
			updatedMetadata = append(updatedMetadata, metadataToRemove)
		}
	}

	data, err = json.MarshalIndent(updatedMetadata, "", "  ")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Error("Failed to marshal updated metadata")
		return
	}

	err = ioutil.WriteFile(metadataFile, data, 0644)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"metadataFile": metadataFile,
			"error":        err,
		}).Error("Failed to write updated metadata")
	} else {
		logrus.WithFields(logrus.Fields{
			"metadataFile": metadataFile,
		}).Info("Metadata successfully updated")
	}
}

func writeMetadata(dateDir string, metadata FileMetadata) error {
	metadataFile := filepath.Join(dateDir, "metadata.json")
	var existingMetadata []FileMetadata

	// Read existing metadata if the file exists
	if _, err := os.Stat(metadataFile); !os.IsNotExist(err) {
		data, err := ioutil.ReadFile(metadataFile)
		if err != nil {
			return err
		}
		err = json.Unmarshal(data, &existingMetadata)
		if err != nil {
			return err
		}
	}

	// Append new metadata
	existingMetadata = append(existingMetadata, metadata)

	// Write updated metadata to file
	data, err := json.MarshalIndent(existingMetadata, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(metadataFile, data, 0644)
}

func readConfig(filePath string) (Config, error) {
	var config Config
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(data, &config)
	if err != nil {
		return config, err
	}
	return config, nil
}

func getServerInfo() (string, string, error) {
	// Get server's private IP
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", "", err
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			return "", "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if !ip.IsLoopback() && ip.To4() != nil {
				hostname, err := os.Hostname()
				if err != nil {
					return "", "", err
				}
				return ip.String(), hostname, nil
			}
		}
	}
	return "", "", fmt.Errorf("no private IP found")
}

func printHelp() {
	fmt.Println("recycler-cli - A safer alternative to rm, moving files to a recycle bin.")
	fmt.Println("---------------------------------------------------------------")
	fmt.Println("Usage:")
	fmt.Println("  recycler-cli [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -f, --files       Comma-separated list of files to recycle (e.g., file1.txt,file2.log)")
	fmt.Println("  -r, --restore     Restore files from recycle bin")
	fmt.Println("  -d, --date        Date to restore files from (format: YYYY-MM-DD)")
	fmt.Println("  -s, --single-file Specify a single file to restore from the recycle bin on a given date")
	fmt.Println("  -h, --help        Display this help message")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  recycler-cli -f file1.txt,file2.log,file3.pdf")
	fmt.Println("  recycler-cli -r -d 2024-11-02")
	fmt.Println("  recycler-cli -r -d 2024-11-02 -s file1.txt")
	fmt.Println()
	fmt.Println("Important:")
	fmt.Println("  - Ensure the recycle bin directory is set to a valid path.")
	fmt.Println("  - Be cautious with file paths to avoid unintended actions.")
}

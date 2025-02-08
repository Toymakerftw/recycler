package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/sirupsen/logrus"
)

func recycleFiles(files []string, serverDir string, numWorkers int, ip string, hostname string, forceRemove bool) {
	// Use structured logging
	logger := logrus.WithFields(logrus.Fields{
		"server": fmt.Sprintf("%s_%s", ip, hostname),
		"workers": numWorkers,
	})

	fileChan := make(chan string, len(files))
	for _, file := range files {
		fileChan <- strings.TrimSpace(file)
	}
	close(fileChan)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range fileChan {
				if !forceRemove && isDirectory(file) {
					logger.WithField("dir", file).Error("Cannot remove directory without -rf flag")
					continue
				}
				moveFileToRecycleBin(file, serverDir, ip, hostname)
			}
		}()
	}
	wg.Wait()
	logger.Info("All specified files have been recycled")
}

func isDirectory(file string) bool {
	fileInfo, err := os.Stat(file)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

func safeMove(src, dest string) error {
	// Try a direct rename first
	if err := os.Rename(src, dest); err == nil {
		return nil
	}

	// Fallback to copy-and-delete for cross-filesystem moves
	return crossDeviceCopy(src, dest)
}

func crossDeviceCopy(src, dest string) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Get file info for permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get source file info: %w", err)
	}

	// Create destination file with same permissions
	destFile, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Copy file contents
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Delete the original file
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("failed to delete original file: %w", err)
	}

	return nil
}

func moveFileToRecycleBin(file string, serverDir string, ip string, hostname string) {
	fileInfo, err := os.Stat(file)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.WithFields(logrus.Fields{"file": file, "server": fmt.Sprintf("%s_%s", ip, hostname)}).Warn("File or directory does not exist")
		} else {
			logrus.WithFields(logrus.Fields{"file": file, "error": err}).Error("Failed to get file info")
		}
		return
	}

	// Create the date directory with 0700 permissions
	dateDir := filepath.Join(serverDir, time.Now().Format("2006-01-02"))
	os.MkdirAll(dateDir, 0700)

	var destPath string
	if fileInfo.IsDir() {
		// Handle directory
		logrus.WithFields(logrus.Fields{"dir": file}).Info("Moving directory to recycle bin")

		destPath = filepath.Join(dateDir, fmt.Sprintf("%s_%s", filepath.Base(file), time.Now().Format("15:04:05")))

		err = safeMove(file, destPath)
		if err != nil {
			logrus.WithFields(logrus.Fields{"dir": file, "destPath": destPath, "error": err}).Error("Failed to move directory")
			return
		}

		metadata := FileMetadata{
			OriginalPath: file,
			OriginalName: filepath.Base(file),
			CurrentName:  filepath.Base(destPath),
			DeletedAt:    time.Now(),
			FileType:     "directory", // Set file type as 'directory'
		}
		writeMetadata(dateDir, metadata)
	} else {
		// Handle file
		logrus.WithFields(logrus.Fields{"file": file}).Info("Moving file to recycle bin")

		// Detect MIME type of the file
		mimeType, err := mimetype.DetectFile(file)
		if err != nil {
			logrus.WithFields(logrus.Fields{"file": file, "error": err}).Error("Failed to detect file type")
			return
		}

		// Create destination path for the file
		destPath = filepath.Join(dateDir, fmt.Sprintf("%s_%s%s", fileInfo.Name(), time.Now().Format("15:04:05"), filepath.Ext(file)))

		// Attempt to rename the file
		err = safeMove(file, destPath)
		if err != nil {
			logrus.WithFields(logrus.Fields{"file": file, "destPath": destPath, "error": err}).Error("Failed to move file")
			return
		}

		metadata := FileMetadata{
			OriginalPath: file,
			OriginalName: fileInfo.Name(),
			CurrentName:  filepath.Base(destPath),
			FileSize:     fileInfo.Size(),
			DeletedAt:    time.Now(),
			FileType:     mimeType.String(),
		}
		writeMetadata(dateDir, metadata)
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

			// Attempt to rename the file
			err = os.Rename(currentPath, originalPath)
			if err != nil {
				// If rename fails, try copying the file instead
				data, err := ioutil.ReadFile(currentPath)
				if err != nil {
					logrus.WithFields(logrus.Fields{"file": currentPath, "error": err}).Error("Failed to read file for copy")
					continue
				}

				err = ioutil.WriteFile(originalPath, data, os.ModePerm)
				if err != nil {
					logrus.WithFields(logrus.Fields{"file": originalPath, "error": err}).Error("Failed to write file copy")
					continue
				}

				// Delete the file from the recycle bin after successful copy
				err = os.Remove(currentPath)
				if err != nil {
					logrus.WithFields(logrus.Fields{"file": currentPath, "error": err}).Error("Failed to delete file from recycle bin after copy")
					continue
				}
			}

			logrus.WithFields(logrus.Fields{
				"originalPath": originalPath,
				"currentPath":  currentPath,
			}).Info("File successfully restored")

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

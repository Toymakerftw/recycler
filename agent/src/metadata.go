package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

type FileMetadata struct {
	OriginalPath string    `json:"original_path"`
	OriginalName string    `json:"original_name"`
	CurrentName  string    `json:"current_name"`
	FileSize     int64     `json:"file_size"`
	DeletedAt    time.Time `json:"deleted_at"`
	FileType     string    `json:"file_type"`
}

func lockFile(fd int) error {
	return unix.Flock(fd, unix.LOCK_EX)
}

func unlockFile(fd int) error {
	return unix.Flock(fd, unix.LOCK_UN)
}

func writeMetadata(dateDir string, metadata FileMetadata) error {
	metadataFile := filepath.Join(dateDir, "metadata.json")

	// Open file with exclusive lock
	file, err := os.OpenFile(metadataFile, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to open metadata file: %w", err)
	}
	defer file.Close()

	// Lock the file
	if err := lockFile(int(file.Fd())); err != nil {
		return fmt.Errorf("failed to lock metadata file: %w", err)
	}
	defer unlockFile(int(file.Fd()))

	// Read existing metadata
	var existingMetadata []FileMetadata
	if info, err := file.Stat(); err == nil && info.Size() > 0 {
		if err := json.NewDecoder(file).Decode(&existingMetadata); err != nil {
			return fmt.Errorf("failed to decode existing metadata: %w", err)
		}
	}

	// Append new metadata
	existingMetadata = append(existingMetadata, metadata)

	// Write updated metadata
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek to beginning of file: %w", err)
	}
	if err := json.NewEncoder(file).Encode(existingMetadata); err != nil {
		return fmt.Errorf("failed to encode metadata: %w", err)
	}

	return nil
}

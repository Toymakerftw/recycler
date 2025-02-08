package main

import (
    "os"
    "testing"
    "time"
)

func TestWriteMetadata(t *testing.T) {
    tmpDir := t.TempDir()
    metadata := FileMetadata{
        OriginalPath: "/tmp/testfile",
        OriginalName: "testfile",
        CurrentName:  "testfile_12345",
        DeletedAt:    time.Now(),
    }

    err := writeMetadata(tmpDir, metadata)
    if err != nil {
        t.Fatalf("writeMetadata failed: %v", err)
    }

    // Verify the file was created
    if _, err := os.Stat(filepath.Join(tmpDir, "metadata.json")); os.IsNotExist(err) {
        t.Error("metadata.json was not created")
    }
}
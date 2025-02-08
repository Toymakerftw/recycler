package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/natefinch/lumberjack"
	"github.com/sirupsen/logrus"
)

type MultiWriter struct {
	writers []io.Writer
}

func (mw *MultiWriter) Write(p []byte) (n int, err error) {
	for _, w := range mw.writers {
		n, err = w.Write(p)
		if err != nil {
			return n, err
		}
	}
	return len(p), nil
}

func initLogger(serverDir string) {
	var writers []io.Writer

	// Primary log file
	primaryLog := &lumberjack.Logger{
		Filename:   "/var/log/cbin/cbin.log",
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
	}
	writers = append(writers, primaryLog)

	// Secondary log file
	secondaryLogPath := filepath.Join(serverDir, "cbin.log")
	secondaryLog := &lumberjack.Logger{
		Filename:   secondaryLogPath,
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
	}
	writers = append(writers, secondaryLog)

	multiWriter := &MultiWriter{writers: writers}
	logrus.SetOutput(multiWriter)
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.InfoLevel)

    // Set file permissions to 0600 for log files
    err := os.Chmod("/var/log/cbin/cbin.log", 0600)
    if err != nil {
        logrus.Errorf("Failed to set permissions on /var/log/cbin/cbin.log: %v", err)
    }

    // Ensure the secondary log file exists and set its permissions
    if _, err := os.Stat(secondaryLogPath); os.IsNotExist(err) {
        file, err := os.Create(secondaryLogPath)
        if err != nil {
            logrus.Fatalf("Failed to create secondary log file: %v", err)
        }
        file.Close()
    }
    err = os.Chmod(secondaryLogPath, 0600)
    if err != nil {
        logrus.Errorf("Failed to set permissions on %s: %v", secondaryLogPath, err)
    }
}

func getServerInfo() (string, string, error) {
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
			switch v := addr.(type) {
			case *net.IPNet:
				if !v.IP.IsLoopback() && v.IP.To4() != nil {
					hostname, err := os.Hostname()
					if err != nil {
						return "", "", err
					}
					return v.IP.String(), hostname, nil
				}
			}
		}
	}
	return "", "", fmt.Errorf("no private IP found")
}

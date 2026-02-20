// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package logs

import (
	"fmt"
	"os"
	"path/filepath"
)

// Logging holds the log file handles and paths needed for a running task.
// This information must be stored in the taskHandle so files can be properly
// closed and cleaned up during task destruction.
type Logging struct {
	// StderrPath is the full path to stderr.0 log file in allocDir/alloc/logs/
	StderrPath string
	// TempFifoPath is the temporary FIFO path created by SDK's captureFifoToFile
	TempFifoPath string
	// StderrFile is the open file handle to stderr.0, kept for cleanup
	StderrFile *os.File
}

// SetupLogging creates the necessary log files and directories for a task.
// Returns a Logging struct containing paths and open file handles.
//
// Phase 1: Captures Firecracker daemon logs to allocDir/alloc/logs/stderr.0
// The log FIFO is created by the SDK, but we manage the target log file.
//
// This function:
// 1. Creates allocDir/alloc/logs/ directory
// 2. Opens stderr.0 file for writing (Firecracker daemon logs)
// 3. Defines the temp FIFO path (SDK will create/manage it)
func SetupLogging(allocDir string) (*Logging, error) {
	// Create the standard Nomad logs directory
	logsDir := filepath.Join(allocDir, "alloc", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Open stderr.0 for Firecracker daemon logs
	stderrPath := filepath.Join(logsDir, "stderr.0")
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open stderr log file: %w", err)
	}

	// Define temp FIFO path (SDK will create/manage it)
	tempFifoPath := filepath.Join(allocDir, "logs.fifo")

	return &Logging{
		StderrPath:   stderrPath,
		TempFifoPath: tempFifoPath,
		StderrFile:   stderrFile,
	}, nil
}

// Cleanup closes log files and removes temporary FIFOs.
// Called during task destruction to ensure proper resource cleanup.
//
// This function:
// 1. Closes the file handle to stderr.0
// 2. Removes the temporary FIFO
// 3. Attempts to remove empty logs directory
//
// Errors are logged as warnings; cleanup is best-effort.
func Cleanup(logging *Logging) {
	if logging == nil {
		return
	}

	// Close the file handle
	if logging.StderrFile != nil {
		if err := logging.StderrFile.Close(); err != nil {
			// Log warning but continue cleanup (file may have already closed)
			_ = err // Could pass logger if needed, but keeping minimal for now
		}
	}

	// Remove temporary FIFO (best-effort, may not exist)
	if logging.TempFifoPath != "" {
		_ = os.Remove(logging.TempFifoPath)
	}

	// Try to remove empty logs directory (best-effort)
	if logging.StderrPath != "" {
		logsDir := filepath.Dir(logging.StderrPath)
		_ = os.Remove(logsDir)
	}
}

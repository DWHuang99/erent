// Package logger creates the API's structured console and rotating file logger.
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

const DefaultFilename = "./logs/backend/app.log"

type dualWriter struct {
	console     io.Writer
	file        io.Writer
	errorOutput io.Writer
}

func (writer *dualWriter) Write(value []byte) (int, error) {
	written, consoleErr := writer.console.Write(value)
	if written != len(value) && consoleErr == nil {
		consoleErr = io.ErrShortWrite
	}
	if _, err := writer.file.Write(value); err != nil {
		_, _ = fmt.Fprintf(writer.errorOutput, "logger file write failed: %v\n", err)
	}
	if consoleErr != nil {
		return written, consoleErr
	}
	return len(value), nil
}

// NewLogger validates the file sink before returning a JSON logger. An empty
// filename uses DefaultFilename. The returned closer must be closed on shutdown.
func NewLogger(filename string) (*slog.Logger, io.Closer, error) {
	return newLogger(filename, os.Stdout, os.Stderr)
}

func newLogger(filename string, console, errorOutput io.Writer) (*slog.Logger, io.Closer, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = DefaultFilename
	}
	if console == nil || errorOutput == nil {
		return nil, nil, fmt.Errorf("logger output writers must not be nil")
	}

	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, nil, fmt.Errorf("create log directory %q: %w", directory, err)
	}
	probe, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file %q: %w", filename, err)
	}
	if err := probe.Close(); err != nil {
		return nil, nil, fmt.Errorf("close log file probe %q: %w", filename, err)
	}

	fileWriter := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    100,
		MaxBackups: 10,
		MaxAge:     30,
		Compress:   true,
	}
	writer := &dualWriter{
		console:     console,
		file:        fileWriter,
		errorOutput: errorOutput,
	}
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler), fileWriter, nil
}

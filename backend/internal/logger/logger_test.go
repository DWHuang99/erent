package logger

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLoggerWritesJSONToConsoleAndFile(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "nested", "app.log")
	var console bytes.Buffer
	var errorOutput bytes.Buffer

	applicationLogger, closer, err := newLogger(filename, &console, &errorOutput)
	if err != nil {
		t.Fatalf("newLogger() error = %v", err)
	}
	applicationLogger.Info("logger test", "request_id", "request-1")
	if err := closer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	fileValue, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for name, value := range map[string]string{
		"console": console.String(),
		"file":    string(fileValue),
	} {
		if !strings.Contains(value, `"msg":"logger test"`) || !strings.Contains(value, `"request_id":"request-1"`) {
			t.Fatalf("%s output = %q, want structured log fields", name, value)
		}
	}
	if errorOutput.Len() != 0 {
		t.Fatalf("error output = %q, want empty", errorOutput.String())
	}
}

func TestNewLoggerRejectsUnusableDirectory(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, _, err := newLogger(filepath.Join(parent, "app.log"), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("newLogger() error = nil, want directory validation error")
	}
}

func TestDualWriterReportsFileFailure(t *testing.T) {
	var console bytes.Buffer
	var errorOutput bytes.Buffer
	writer := &dualWriter{
		console:     &console,
		file:        failingWriter{},
		errorOutput: &errorOutput,
	}

	value := []byte("message")
	written, err := writer.Write(value)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if written != len(value) || console.String() != string(value) {
		t.Fatalf("Write() = (%d, %q), want (%d, %q)", written, console.String(), len(value), value)
	}
	if !strings.Contains(errorOutput.String(), "logger file write failed: forced failure") {
		t.Fatalf("error output = %q, want file failure diagnostic", errorOutput.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("forced failure")
}

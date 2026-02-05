package event

import (
	"bufio"
	"fmt"
	"os"
	"sync"
)

// FileSink writes AgentEvents to a JSONL file.
// It is thread-safe and append-only.
type FileSink struct {
	mu     sync.Mutex
	file   *os.File
	writer *bufio.Writer
	path   string
}

// NewFileSink creates a new FileSink that writes to the specified file path.
// The file is created if it doesn't exist, or appended to if it does.
func NewFileSink(path string) (*FileSink, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open event file: %w", err)
	}

	return &FileSink{
		file:   file,
		writer: bufio.NewWriter(file),
		path:   path,
	}, nil
}

// Write writes a single event to the JSONL file.
func (s *FileSink) Write(event *AgentEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := event.MarshalJSONL()
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if _, err := s.writer.Write(data); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}
	if err := s.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// WriteBatch writes multiple events to the JSONL file.
func (s *FileSink) WriteBatch(events []*AgentEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, event := range events {
		data, err := event.MarshalJSONL()
		if err != nil {
			return fmt.Errorf("failed to marshal event: %w", err)
		}

		if _, err := s.writer.Write(data); err != nil {
			return fmt.Errorf("failed to write event: %w", err)
		}
		if err := s.writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("failed to write newline: %w", err)
		}
	}

	return nil
}

// Flush flushes any buffered data to the underlying file.
func (s *FileSink) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}
	return nil
}

// Close flushes and closes the underlying file.
func (s *FileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}
	if err := s.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}
	return nil
}

// Path returns the file path of the sink.
func (s *FileSink) Path() string {
	return s.path
}

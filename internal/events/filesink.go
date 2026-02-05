package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileSink writes AgentEvents to a JSONL file for local debugging.
// It is safe for concurrent use from multiple goroutines.
type FileSink struct {
	path   string
	file   *os.File
	writer *bufio.Writer
	mu     sync.Mutex
}

// DefaultFilename is the default filename for the events file.
const DefaultFilename = "events.jsonl"

// NewFileSink creates a new FileSink that writes to the specified directory.
// The events file will be created at dir/events.jsonl.
// If the file already exists, new events will be appended.
func NewFileSink(dir string) (*FileSink, error) {
	path := filepath.Join(dir, DefaultFilename)

	// Open file in append mode, create if not exists
	// Use 0600 permissions for security (potential sensitive tool inputs/results)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open events file: %w", err)
	}

	return &FileSink{
		path:   path,
		file:   file,
		writer: bufio.NewWriter(file),
	}, nil
}

// Write writes a batch of events to the JSONL file.
// Each event is written as a single JSON line.
func (s *FileSink) Write(events []AgentEvent) error {
	if len(events) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, event := range events {
		data, err := json.Marshal(event)
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

	// Flush to ensure events are persisted
	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush events: %w", err)
	}

	return nil
}

// WriteOne writes a single event to the JSONL file.
func (s *FileSink) WriteOne(event AgentEvent) error {
	return s.Write([]AgentEvent{event})
}

// Flush flushes any buffered data to the underlying file.
func (s *FileSink) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush buffer: %w", err)
	}
	return nil
}

// Close flushes any remaining data and closes the file.
func (s *FileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file == nil {
		return nil
	}

	// Flush any remaining buffered data
	if err := s.writer.Flush(); err != nil {
		// Still try to close the file even if flush fails
		_ = s.file.Close()
		s.file = nil
		return fmt.Errorf("failed to flush before close: %w", err)
	}

	if err := s.file.Close(); err != nil {
		s.file = nil
		return fmt.Errorf("failed to close events file: %w", err)
	}

	s.file = nil
	return nil
}

// Path returns the path to the events file.
func (s *FileSink) Path() string {
	return s.path
}

// ReadEvents reads all events from a JSONL file.
// This is useful for testing and analysis.
func ReadEvents(path string) ([]AgentEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open events file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var events []AgentEvent
	scanner := bufio.NewScanner(file)

	// Set a larger buffer for potentially large JSON lines (1MB max)
	const maxLineSize = 1024 * 1024
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event AgentEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("failed to parse event on line %d: %w", lineNum, err)
		}
		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read events file: %w", err)
	}

	return events, nil
}

// FilterByType filters events by event type.
func FilterByType(events []AgentEvent, types ...EventType) []AgentEvent {
	if len(types) == 0 {
		return events
	}

	typeSet := make(map[EventType]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	var filtered []AgentEvent
	for _, event := range events {
		if typeSet[event.Type] {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// FilterByIteration filters events by iteration number.
// If iteration is 0 or negative, all events are returned.
func FilterByIteration(events []AgentEvent, iteration int) []AgentEvent {
	if iteration <= 0 {
		return events
	}

	var filtered []AgentEvent
	for _, event := range events {
		if event.Iteration == iteration {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

package artifacts

import (
	"fmt"
	"os"
	"sync"
)

var discarder = &discardCloser{}

// A discardCloser is an io.WriteCloser that discards all data written to it.
// It is used when no artifact directory is configured, to provide a no-op
// writer that never fails.
type discardCloser struct{}

func (d *discardCloser) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (d *discardCloser) Close() error {
	return nil
}

// openStreamManager manages open streams for artifact writing.
type openStreamManager struct {
	mu      sync.Mutex
	streams map[string]*artifactStream
}

func (m *openStreamManager) newStream(destination string) (*artifactStream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.streams == nil {
		m.streams = make(map[string]*artifactStream)
	}

	if _, exists := m.streams[destination]; exists {
		return nil, fmt.Errorf("artifact stream for destination '%s' is already open", destination)
	}

	file, err := os.Create(destination)
	if err != nil {
		return nil, fmt.Errorf("failed to create artifact file '%s': %w", destination, err)
	}

	stream := &artifactStream{
		destination: destination,
		file:        file,
		manager:     m,
	}

	m.streams[destination] = stream

	return stream, nil
}

func (m *openStreamManager) closeStream(destination string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.streams[destination]
	if !exists {
		return
	}

	delete(m.streams, destination)
}

type artifactStream struct {
	destination string
	file        *os.File
	manager     *openStreamManager
}

func (s *artifactStream) Write(p []byte) (n int, err error) {
	return s.file.Write(p)
}

func (s *artifactStream) Close() error {
	s.manager.closeStream(s.destination)
	return s.file.Close()
}

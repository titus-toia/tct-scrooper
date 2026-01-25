package logging

import (
	"io"
	"log"
	"os"
	"sync"
)

const maxLogSize = 2 * 1024 * 1024 // 2MB

type RotatingWriter struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	size    int64
	maxSize int64
}

func Setup(logPath string) (*RotatingWriter, error) {
	// Truncate if too large on startup
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxLogSize {
		os.Truncate(logPath, 0)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	info, _ := f.Stat()
	size := int64(0)
	if info != nil {
		size = info.Size()
	}

	rw := &RotatingWriter{
		file:    f,
		path:    logPath,
		size:    size,
		maxSize: maxLogSize,
	}

	multi := io.MultiWriter(os.Stdout, rw)
	log.SetOutput(multi)

	return rw, nil
}

func (w *RotatingWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n, err = w.file.Write(p)
	w.size += int64(n)

	if w.size > w.maxSize {
		w.rotate()
	}

	return n, err
}

func (w *RotatingWriter) rotate() {
	w.file.Close()

	// Keep one backup
	os.Rename(w.path, w.path+".1")

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return
	}

	w.file = f
	w.size = 0
}

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditLogRoot is the default directory for append-only audit log files.
const AuditLogRoot = "/app/data/audit"

// FileSink writes audit events to append-only daily-rotated files.
type FileSink struct {
	mu      sync.Mutex
	root    string
	current *os.File
	curDate string
	// NowFn returns the current time. Override in tests for deterministic rotation.
	NowFn func() time.Time
}

// NewFileSink creates a new FileSink that writes to the given root directory.
func NewFileSink(root string) *FileSink {
	os.MkdirAll(root, 0755)
	return &FileSink{root: root, NowFn: time.Now}
}

// Write appends a JSON event line to today's audit file.
func (fs *FileSink) Write(event map[string]interface{}) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	today := fs.NowFn().Format("2006-01-02")

	// Rotate if needed
	if today != fs.curDate {
		if fs.current != nil {
			// Seal previous file as read-only
			name := fs.current.Name()
			fs.current.Close()
			os.Chmod(name, 0444)
		}
		path := filepath.Join(fs.root, fmt.Sprintf("audit-%s.jsonl", today))
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("audit file: open: %w", err)
		}
		fs.current = f
		fs.curDate = today
	}

	line, _ := json.Marshal(event)
	line = append(line, '\n')
	_, err := fs.current.Write(line)
	return err
}

// Close closes the current audit file.
func (fs *FileSink) Close() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.current != nil {
		fs.current.Close()
	}
}

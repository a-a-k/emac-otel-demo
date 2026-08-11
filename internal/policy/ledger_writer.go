package policy

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/a-a-k/emac-otel-demo/internal/ledger"
)

type LedgerWriter struct {
	mu   sync.Mutex
	file *os.File
}

func OpenLedger(path string) (*LedgerWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	// The experiment runner must reconcile and archive this bind-mounted file.
	// The directory contains no credentials or user data.
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &LedgerWriter{file: f}, nil
}
func (w *LedgerWriter) Write(v ledger.Request) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := json.NewEncoder(w.file).Encode(v); err != nil {
		return err
	}
	return w.file.Sync()
}
func (w *LedgerWriter) Close() error { w.mu.Lock(); defer w.mu.Unlock(); return w.file.Close() }

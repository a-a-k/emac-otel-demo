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

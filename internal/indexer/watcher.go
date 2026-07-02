package indexer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Watcher polls the project tree for changes and re-indexes modified files.
// We use polling (not fsnotify) to stay stdlib-only and cross-platform.
// The poll interval is a trade-off: shorter = more responsive, more CPU.
//
// Usage:
//   w := NewWatcher(indexer, 5*time.Second)
//   w.Start()
//   defer w.Stop()
type Watcher struct {
	indexer  *Indexer
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}

	mu       sync.Mutex
	modTimes map[string]time.Time // file path → last known mod time
	running  bool
}

// NewWatcher creates a watcher that polls every interval.
func NewWatcher(ix *Indexer, interval time.Duration) *Watcher {
	return &Watcher{
		indexer:  ix,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		modTimes: make(map[string]time.Time),
	}
}

// Start begins polling in a background goroutine. Returns immediately.
// Calling Start twice panics.
func (w *Watcher) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		panic("watcher already started")
	}
	w.running = true
	w.mu.Unlock()
	go w.run()
}

// Stop signals the polling goroutine to exit and blocks until it does.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()
	close(w.stop)
	<-w.done
	w.mu.Lock()
	w.running = false
	w.mu.Unlock()
}

func (w *Watcher) run() {
	defer close(w.done)
	// Snapshot mod times on first run.
	w.scan(true)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			w.scan(false)
		}
	}
}

// scan walks the tree looking for changed/new/deleted files. If initial,
// it records mod times without re-indexing (Build already indexed them).
func (w *Watcher) scan(initial bool) {
	seen := make(map[string]bool)
	_ = filepath.WalkDir(w.indexer.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel := w.indexer.rel(path)
			if isIgnoredDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !isSupportedExt(ext) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mtime := info.ModTime()
		seen[path] = true

		w.mu.Lock()
		old, exists := w.modTimes[path]
		w.mu.Unlock()

		if !exists {
			// New file.
			w.mu.Lock()
			w.modTimes[path] = mtime
			w.mu.Unlock()
			if !initial {
				_ = w.indexer.ReindexFile(path)
			}
		} else if !mtime.Equal(old) {
			// Changed file.
			w.mu.Lock()
			w.modTimes[path] = mtime
			w.mu.Unlock()
			_ = w.indexer.ReindexFile(path)
		}
		return nil
	})

	// Detect deleted files: in our map but not seen this scan.
	w.mu.Lock()
	for path := range w.modTimes {
		if !seen[path] {
			delete(w.modTimes, path)
			rel := w.indexer.rel(path)
			w.indexer.index.ClearFile(rel)
		}
	}
	w.mu.Unlock()
}

func isSupportedExt(ext string) bool {
	switch ext {
	case ".py", ".go":
		return true
	}
	return false
}

// silence unused import in some build configs
var _ = os.Stat

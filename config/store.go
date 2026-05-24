package config

import (
	"context"
	"log"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// RuleStore holds the active rule set and supports hot reload from a YAML file.
type RuleStore struct {
	path string
	val  atomic.Value // *RuleSet
}

// NewRuleStore loads and validates rules from path, then enables file watching via Watch.
func NewRuleStore(path string) (*RuleStore, error) {
	rs, err := loadAndValidate(path)
	if err != nil {
		return nil, err
	}
	s := &RuleStore{path: path}
	s.val.Store(rs)
	return s, nil
}

// NewRuleStoreFromRules creates an in-memory store (no file watching). For tests.
func NewRuleStoreFromRules(rules *RuleSet) *RuleStore {
	s := &RuleStore{}
	s.val.Store(rules)
	return s
}

// Get returns the current rule set. Do not mutate the returned value.
func (s *RuleStore) Get() *RuleSet {
	return s.val.Load().(*RuleSet)
}

// Reload reads the config file again. On validation failure the previous config is kept.
func (s *RuleStore) Reload() error {
	if s.path == "" {
		return nil
	}
	rs, err := loadAndValidate(s.path)
	if err != nil {
		return err
	}
	s.val.Store(rs)
	log.Printf("Rule config reloaded from %s", s.path)
	return nil
}

// Watch monitors the config file and reloads on change until ctx is cancelled.
func (s *RuleStore) Watch(ctx context.Context) {
	if s.path == "" {
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Warning: failed to start rule config watcher: %v", err)
		return
	}
	defer watcher.Close()

	dir := filepath.Dir(s.path)
	target := filepath.Base(s.path)
	if err := s.addWatchPaths(watcher, dir); err != nil {
		log.Printf("Warning: failed to watch rule config at %s: %v", s.path, err)
		return
	}
	log.Printf("Watching rule config for changes: %s", s.path)

	var debounce <-chan time.Time
	var debounceTimer *time.Timer

	reload := func() {
		if err := s.Reload(); err != nil {
			log.Printf("Rule config reload failed (keeping previous config): %v", err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		case <-debounce:
			debounce = nil
			reload()
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Base(event.Name) != target {
				continue
			}
			if event.Op&fsnotify.Remove != 0 {
				_ = s.addWatchPaths(watcher, dir)
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Chmod) == 0 {
				continue
			}
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.NewTimer(200 * time.Millisecond)
			debounce = debounceTimer.C
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Rule config watcher error: %v", err)
		}
	}
}

func (s *RuleStore) addWatchPaths(watcher *fsnotify.Watcher, dir string) error {
	if err := watcher.Add(dir); err != nil {
		return err
	}
	if err := watcher.Add(s.path); err != nil {
		// File-level watch may fail on some platforms; directory watch is enough.
		log.Printf("Note: file-level watch unavailable for %s: %v", s.path, err)
	}
	return nil
}

func loadAndValidate(path string) (*RuleSet, error) {
	rs, err := LoadRuleSet(path)
	if err != nil {
		return nil, err
	}
	if err := ValidateRuleSet(rs); err != nil {
		return nil, err
	}
	return rs, nil
}

// Package indexer keeps the SQLite index in sync with transcript files by
// parsing only bytes appended since the last pass.
package indexer

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/store"
)

type Progress struct {
	Done, Total int
	Changed     int
}

type Indexer struct {
	Store     *store.Store
	ClaudeDir string
}

type job struct {
	path  string
	info  os.FileInfo
	prev  store.FileState
	known bool
}

type result struct {
	sess   *claude.Session
	fs     store.FileState
	chunks []claude.Chunk
	reset  bool
	err    error
}

// Sync indexes new and changed files. progress, if non-nil, is called from the
// calling goroutine after each file is written.
func (ix *Indexer) Sync(ctx context.Context, progress func(Progress)) (Progress, error) {
	files, err := claude.SessionFiles(ix.ClaudeDir)
	if err != nil {
		return Progress{}, err
	}
	states, err := ix.Store.FileStates()
	if err != nil {
		return Progress{}, err
	}
	var jobs []job
	seen := map[string]bool{}
	for _, f := range files {
		seen[f] = true
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		prev, known := states[f]
		if known && prev.Size == info.Size() && prev.Mtime == info.ModTime().UnixNano() {
			continue
		}
		jobs = append(jobs, job{path: f, info: info, prev: prev, known: known})
	}
	for path := range states {
		if !seen[path] {
			_ = ix.Store.RemovePath(path)
		}
	}
	// Big files first so the pool stays busy until the end.
	sortBySizeDesc(jobs)

	p := Progress{Total: len(jobs)}
	if len(jobs) == 0 {
		return p, nil
	}
	in := make(chan job)
	out := make(chan result)
	var wg sync.WaitGroup
	for range min(runtime.NumCPU(), 8) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range in {
				out <- parse(j)
			}
		}()
	}
	go func() {
		defer close(in)
		for _, j := range jobs {
			select {
			case in <- j:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(out) }()

	var firstErr error
	for r := range out {
		p.Done++
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		if r.sess.ID == "" {
			continue
		}
		if err := ix.Store.Save(r.sess, r.fs, r.chunks, r.reset); err != nil && firstErr == nil {
			firstErr = err
		}
		p.Changed++
		if progress != nil {
			progress(p)
		}
	}
	if ctx.Err() != nil {
		return p, ctx.Err()
	}
	return p, firstErr
}

func parse(j job) result {
	f, err := os.Open(j.path)
	if err != nil {
		return result{err: err}
	}
	defer f.Close()

	resume := j.known && j.info.Size() >= j.prev.Offset && j.prev.Session != nil
	var sess *claude.Session
	var offset int64
	seq := 0
	if resume {
		cp := *j.prev.Session
		sess = &cp
		offset = j.prev.Offset
		seq = j.prev.Seq
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return result{err: err}
		}
	} else {
		sess = &claude.Session{}
	}
	sess.Path = j.path
	sess.Project = filepath.Base(filepath.Dir(j.path))
	p := claude.NewParser(sess, seq)
	n, err := p.Feed(f)
	if err != nil {
		return result{err: err}
	}
	if sess.ID == "" {
		sess.ID = strings.TrimSuffix(filepath.Base(j.path), ".jsonl")
	}
	return result{
		sess:   sess,
		fs:     store.FileState{Size: j.info.Size(), Mtime: j.info.ModTime().UnixNano(), Offset: offset + n, Seq: p.Seq},
		chunks: p.Chunks,
		reset:  !resume,
	}
}

func sortBySizeDesc(js []job) {
	sort.Slice(js, func(a, b int) bool { return js[a].info.Size() > js[b].info.Size() })
}

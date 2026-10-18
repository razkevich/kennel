// Package indexer keeps the SQLite index in sync with transcript files by
// parsing only bytes appended since the last pass.
package indexer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/cursor"
	"github.com/razkevich/kennel/internal/store"
)

type Progress struct {
	Done, Total int
	Changed     int
}

type Indexer struct {
	// mu serializes syncs in this process; SaveBatch guards across processes.
	mu        sync.Mutex
	Store     *store.Store
	ClaudeDir string
	// CodexDir, when set, also indexes Codex CLI rollouts.
	CodexDir string
	// CursorDir, when set, also indexes Cursor composer chats.
	CursorDir string
}

type job struct {
	path  string
	info  os.FileInfo
	prev  store.FileState
	known bool
}

type result struct {
	prev   int64
	sess   *claude.Session
	fs     store.FileState
	chunks []claude.Chunk
	reset  bool
	err    error
}

// Sync indexes new and changed files. progress, if non-nil, is called from the
// calling goroutine after each file is written.
func (ix *Indexer) Sync(ctx context.Context, progress func(Progress)) (Progress, error) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	files, err := claude.SessionFiles(ix.ClaudeDir)
	if err != nil {
		return Progress{}, err
	}
	if ix.CodexDir != "" {
		files = append(files, claude.CodexFiles(ix.CodexDir)...)
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
		if !seen[path] && !strings.HasPrefix(path, "cursor:") {
			_ = ix.Store.RemovePath(path)
		}
	}
	cursorChanged := ix.syncCursor(states)
	// Big files first so the pool stays busy until the end.
	sortBySizeDesc(jobs)

	p := Progress{Total: len(jobs), Changed: cursorChanged}
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
	var batch []store.SaveItem
	chunks := 0
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := ix.Store.SaveBatch(batch); err != nil && firstErr == nil {
			firstErr = err
		}
		p.Changed += len(batch)
		batch, chunks = batch[:0], 0
		if progress != nil {
			progress(p)
		}
	}
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
		batch = append(batch, store.SaveItem{Session: r.sess, File: r.fs, Chunks: r.chunks, Reset: r.reset, PrevOffset: r.prev})
		chunks += len(r.chunks)
		if len(batch) >= 64 || chunks >= 20000 {
			flush()
		}
	}
	flush()
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
	if strings.HasPrefix(filepath.Base(j.path), "rollout-") {
		p.Codex = true
		sess.Agent = "codex"
		sess.Project = "codex"
	}
	n, err := p.Feed(f)
	if err != nil {
		return result{err: err}
	}
	// Claude resumes by the file's UUID, and copied lines in forks can carry a
	// parent's sessionId, so the file name is the identity.
	if !p.Codex {
		sess.ID = strings.TrimSuffix(filepath.Base(j.path), ".jsonl")
	} else if sess.ID == "" {
		sess.ID = strings.TrimSuffix(filepath.Base(j.path), ".jsonl")
		if len(sess.ID) > 36 {
			sess.ID = sess.ID[len(sess.ID)-36:]
		}
	}
	prev := int64(-1)
	if resume {
		prev = j.prev.Offset
	}
	return result{
		prev:   prev,
		sess:   sess,
		fs:     store.FileState{Size: j.info.Size(), Mtime: j.info.ModTime().UnixNano(), Offset: offset + n, Seq: p.Seq},
		chunks: p.Chunks,
		reset:  !resume,
	}
}

func sortBySizeDesc(js []job) {
	sort.Slice(js, func(a, b int) bool { return js[a].info.Size() > js[b].info.Size() })
}

// syncCursor indexes Cursor composer chats whose lastUpdatedAt moved. Cursor
// isn't file-per-session, so the database mtime gates the (cheap) scan.
func (ix *Indexer) syncCursor(states map[string]store.FileState) int {
	if ix.CursorDir == "" {
		return 0
	}
	db := cursor.GlobalDB(ix.CursorDir)
	info, err := os.Stat(db)
	if err != nil {
		return 0
	}
	mt := fmt.Sprint(info.ModTime().UnixNano())
	if wal, err := os.Stat(db + "-wal"); err == nil {
		mt += "/" + fmt.Sprint(wal.ModTime().UnixNano(), wal.Size())
	}
	if ix.Store.Get("cursor_mtime") == mt {
		return 0
	}
	since := map[string]int64{}
	for path, fs := range states {
		if id, ok := strings.CutPrefix(path, "cursor:"); ok {
			since[id] = fs.Mtime
		}
	}
	convs, all, err := cursor.Read(ix.CursorDir, since)
	if err != nil {
		return 0
	}
	var batch []store.SaveItem
	for _, c := range convs {
		ms := c.Session.Updated.UnixMilli()
		batch = append(batch, store.SaveItem{Session: c.Session, File: store.FileState{Size: ms, Mtime: ms, Offset: ms, Seq: len(c.Chunks)}, Chunks: c.Chunks, Reset: true})
	}
	if len(batch) > 0 {
		if err := ix.Store.SaveBatch(batch); err != nil {
			return 0
		}
	}
	for path := range states {
		if id, ok := strings.CutPrefix(path, "cursor:"); ok && !all[id] {
			_ = ix.Store.RemovePath(path)
		}
	}
	_ = ix.Store.Set("cursor_mtime", mt)
	return len(batch)
}

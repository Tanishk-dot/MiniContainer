package unionfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type nodeState struct {
	source  string
	deleted bool
}

// merger tracks the unified view of stacked read-only and writable layers.
type merger struct {
	nodes map[string]nodeState
}

func newMerger() *merger {
	return &merger{nodes: make(map[string]nodeState)}
}

func normalizeRel(rel string) (string, error) {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." {
		return "", nil
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "../") || rel == ".." {
		return "", ErrInvalidPath
	}
	return rel, nil
}

func (m *merger) applyLayer(layerRoot string) error {
	layerRoot = filepath.Clean(layerRoot)
	info, err := os.Stat(layerRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("unionfs: layer root is not a directory: %s", layerRoot)
	}

	entries, err := collectLayerEntries(layerRoot)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		switch entry.kind {
		case entryOpaque:
			m.applyOpaque(entry.dir)
		case entryWhiteout:
			m.applyWhiteout(entry.dir, entry.target)
		case entryFile:
			m.applyNode(entry.rel, entry.source)
		case entryDir:
			m.applyNode(entry.rel, entry.source)
		}
	}
	return nil
}

type entryKind int

const (
	entryFile entryKind = iota
	entryDir
	entryWhiteout
	entryOpaque
)

type layerEntry struct {
	kind   entryKind
	rel    string
	dir    string
	target string
	source string
}

func collectLayerEntries(layerRoot string) ([]layerEntry, error) {
	var entries []layerEntry
	var opaques []layerEntry
	var whiteouts []layerEntry
	var nodes []layerEntry

	err := filepath.WalkDir(layerRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(layerRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		nrel, err := normalizeRel(rel)
		if err != nil {
			return err
		}

		name := d.Name()
		dir := filepath.ToSlash(filepath.Dir(nrel))
		if dir == "." {
			dir = ""
		}

		if d.IsDir() {
			nodes = append(nodes, layerEntry{
				kind:   entryDir,
				rel:    nrel,
				source: path,
			})
			return nil
		}

		if IsOpaqueWhiteout(name) {
			opaques = append(opaques, layerEntry{kind: entryOpaque, dir: dir})
			return nil
		}
		if target, ok := WhiteoutTarget(name); ok {
			whiteouts = append(whiteouts, layerEntry{
				kind:   entryWhiteout,
				dir:    dir,
				target: target,
			})
			return nil
		}

		nodes = append(nodes, layerEntry{
			kind:   entryFile,
			rel:    nrel,
			source: path,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Apply whiteouts and opaque markers before regular nodes within a layer.
	entries = append(entries, opaques...)
	entries = append(entries, whiteouts...)
	entries = append(entries, nodes...)
	return entries, nil
}

func (m *merger) applyOpaque(dir string) {
	dir = filepath.ToSlash(filepath.Clean(dir))
	if dir == "." {
		dir = ""
	}

	prefix := dir
	if prefix != "" {
		prefix += "/"
	}

	for key := range m.nodes {
		if dir == "" {
			delete(m.nodes, key)
			continue
		}
		if key == dir || strings.HasPrefix(key, prefix) {
			delete(m.nodes, key)
		}
	}
}

func (m *merger) applyWhiteout(dir, target string) {
	key := target
	if dir != "" {
		key = dir + "/" + target
	}
	m.nodes[key] = nodeState{deleted: true}
}

func (m *merger) applyNode(rel, source string) {
	rel, err := normalizeRel(rel)
	if err != nil {
		return
	}
	m.nodes[rel] = nodeState{source: source}
}

func (m *merger) materialize(mergedRoot string) error {
	if err := os.RemoveAll(mergedRoot); err != nil {
		return fmt.Errorf("unionfs: reset merged root: %w", err)
	}
	if err := os.MkdirAll(mergedRoot, 0o755); err != nil {
		return fmt.Errorf("unionfs: create merged root: %w", err)
	}

	for rel, state := range m.nodes {
		if state.deleted || state.source == "" {
			continue
		}

		dest := filepath.Join(mergedRoot, rel)
		info, err := os.Stat(state.source)
		if err != nil {
			return fmt.Errorf("unionfs: stat %s: %w", state.source, err)
		}

		if info.IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return fmt.Errorf("unionfs: mkdir merged dir %s: %w", dest, err)
			}
			continue
		}

		if err := linkOrCopy(state.source, dest); err != nil {
			return fmt.Errorf("unionfs: materialize %s: %w", rel, err)
		}
	}
	return nil
}

func buildMergedView(mergedRoot string, readOnlyLayers []string, writableLayer string) error {
	merger := newMerger()

	for _, layerRoot := range readOnlyLayers {
		if err := merger.applyLayer(layerRoot); err != nil {
			return err
		}
	}
	if writableLayer != "" {
		if err := merger.applyLayer(writableLayer); err != nil {
			return err
		}
	}
	return merger.materialize(mergedRoot)
}

func linkOrCopy(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	_ = os.RemoveAll(dst)

	if err := os.Link(src, dst); err == nil {
		return nil
	}
	if err := os.Symlink(src, dst); err == nil {
		return nil
	}
	return copyPath(src, dst)
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

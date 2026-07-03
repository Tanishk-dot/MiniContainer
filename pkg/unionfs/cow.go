package unionfs

import (
	"fmt"
	"os"
	"path/filepath"
)

// findInLayers searches for relPath in writable first, then read-only layers top-down.
func findInLayers(writable string, readOnly []string, relPath string) (string, int, error) {
	relPath, err := normalizeRel(relPath)
	if err != nil {
		return "", -1, err
	}

	if writable != "" {
		path := filepath.Join(writable, relPath)
		if _, err := os.Lstat(path); err == nil {
			return path, len(readOnly), nil
		} else if !os.IsNotExist(err) {
			return "", -1, err
		}
	}

	for i := len(readOnly) - 1; i >= 0; i-- {
		path := filepath.Join(readOnly[i], relPath)
		if _, err := os.Lstat(path); err == nil {
			return path, i, nil
		} else if !os.IsNotExist(err) {
			return "", -1, err
		}
	}
	return "", -1, ErrNotFound
}

// isWhiteouted checks whether relPath is hidden by a whiteout in writable or upper layers.
func isWhiteouted(writable string, readOnly []string, relPath string) (bool, error) {
	relPath, err := normalizeRel(relPath)
	if err != nil {
		return false, err
	}

	dir := filepath.ToSlash(filepath.Dir(relPath))
	if dir == "." {
		dir = ""
	}
	name := filepath.Base(relPath)

	layers := append(append([]string{}, readOnly...), writable)
	for i := len(layers) - 1; i >= 0; i-- {
		if layers[i] == "" {
			continue
		}
		layerRoot := layers[i]

		whiteoutPath := filepath.Join(layerRoot, dir, WhiteoutPrefix+name)
		if _, err := os.Lstat(whiteoutPath); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
	}
	return false, nil
}

// copyUp copies relPath from lower layers into the writable layer when absent there.
func copyUp(writable string, readOnly []string, relPath string) error {
	if writable == "" {
		return fmt.Errorf("unionfs: no writable layer configured")
	}

	relPath, err := normalizeRel(relPath)
	if err != nil {
		return err
	}

	dest := filepath.Join(writable, relPath)
	if _, err := os.Lstat(dest); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	hidden, err := isWhiteouted(writable, readOnly, relPath)
	if err != nil {
		return err
	}
	if hidden {
		return ErrNotFound
	}

	src, _, err := findInLayers("", readOnly, relPath)
	if err != nil {
		return err
	}

	return copyPath(src, dest)
}

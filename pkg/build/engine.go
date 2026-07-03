package build

import (
    "archive/tar"
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "time"

    "cloudforge/pkg/hash"
    "cloudforge/pkg/layer"
)

// Engine builds layers from deterministic steps using a layer.Manager for storage.
type Engine struct {
    manager layer.Manager
}

// NewEngine creates a new build engine.
func NewEngine(m layer.Manager) *Engine {
    return &Engine{manager: m}
}

// Build executes the given steps and returns the ordered produced results (bottom->top).
func (e *Engine) Build(ctx context.Context, steps []Step) ([]Result, error) {
    var results []Result
    var parent *hash.Digest

    for _, step := range steps {
        if err := ctx.Err(); err != nil {
            return nil, err
        }
        switch step.Type {
        case StepAdd:
            // create deterministic tar archive bytes
            tarBytes, err := createTar(step.Path, step.Content)
            if err != nil {
                return nil, fmt.Errorf("build: create tar: %w", err)
            }
            // compute expected digest of content
            expected := hash.FromBytes(tarBytes)

            // If layer already exists, reuse it
            if existing, err := e.manager.Get(ctx, expected); err == nil {
                results = append(results, Result{ID: existing.ID, Size: existing.Size, MediaType: existing.MediaType, CreatedAt: existing.CreatedAt})
                parent = &existing.ID
                continue
            } else if err != layer.ErrNotFound {
                return nil, err
            }

            // Store new layer with parent chain
            stored, err := e.manager.Store(ctx, tarBytes, parent)
            if err != nil {
                return nil, fmt.Errorf("build: store layer: %w", err)
            }
            results = append(results, Result{ID: stored.ID, Size: stored.Size, MediaType: stored.MediaType, CreatedAt: stored.CreatedAt})
            parent = &stored.ID

        default:
            return nil, fmt.Errorf("build: unsupported step type %q", step.Type)
        }
    }
    return results, nil
}

// createTar creates a minimal deterministic tar containing a single file at path with given content.
func createTar(path string, content []byte) ([]byte, error) {
    buf := &bytes.Buffer{}
    tw := tar.NewWriter(buf)
    hdr := &tar.Header{
        Name:    path,
        Mode:    0644,
        Size:    int64(len(content)),
        ModTime: time.Unix(0, 0).UTC(),
    }
    if err := tw.WriteHeader(hdr); err != nil {
        return nil, err
    }
    if _, err := io.Copy(tw, bytes.NewReader(content)); err != nil {
        return nil, err
    }
    if err := tw.Close(); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}

// helper to compute a deterministic key for a step (not currently used but provided).
func stepKey(step Step, parent *hash.Digest) ([]byte, error) {
    copy := struct {
        Step   Step   `json:"step"`
        Parent *string `json:"parent,omitempty"`
    }{Step: step}
    if parent != nil {
        s := parent.String()
        copy.Parent = &s
    }
    return json.Marshal(copy)
}

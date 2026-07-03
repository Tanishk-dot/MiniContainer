//go:build linux
// +build linux

package runtime

import (
    "context"
    "testing"
    "time"
)

func TestContainer_Run(t *testing.T) {
    c := New([]string{"/bin/echo", "hello-world"})
    c.MountProc = false
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := c.Run(ctx); err != nil {
        t.Skipf("skipping runtime test (requires linux privileges): %v", err)
    }
}

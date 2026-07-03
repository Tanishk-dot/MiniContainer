//go:build linux
// +build linux

package runtime

import (
    "context"
    "os"
    "testing"
    "time"
)

func TestContainer_CgroupLimits(t *testing.T) {
    // Quick capability check: attempt to create a temp dir under /sys/fs/cgroup.
    tmp, err := os.MkdirTemp("/sys/fs/cgroup", "cf-test-")
    if err != nil {
        t.Skipf("skipping cgroup test; cannot create cgroup dir: %v", err)
    }
    _ = os.Remove(tmp)

    c := New([]string{"/bin/echo", "cgtest"})
    c.MountProc = false
    c.Resources = &Resources{PidsLimit: 64, MemoryLimitBytes: 16 * 1024 * 1024, CPUQuotaPercent: 10}

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := c.Run(ctx); err != nil {
        t.Fatalf("container run with cgroup failed: %v", err)
    }
}

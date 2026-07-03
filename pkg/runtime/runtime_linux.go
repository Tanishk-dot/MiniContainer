//go:build linux
// +build linux

package runtime

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "syscall"
    "time"
)

// Run executes the container command in new Linux namespaces.
// Note: this requires appropriate privileges (CAP_SYS_ADMIN / root) for many operations.
func (c *Container) Run(ctx context.Context) error {
    if len(c.Cmd) == 0 {
        return fmt.Errorf("runtime: empty command")
    }

    // Construct a child shell script to perform simple setup (hostname, mount /proc) then exec command.
    var parts []string
    if c.Hostname != "" {
        parts = append(parts, "hostname "+quote(c.Hostname))
    }
    if c.MountProc {
        // best-effort mount; may fail without privileges
        parts = append(parts, "mount -t proc proc /proc || true")
    }
    // exec the desired command
    parts = append(parts, "exec "+joinQuoted(c.Cmd))
    script := strings.Join(parts, " && ")

    cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    // Clone into new namespaces (UTS, PID, MNT, NET). PID namespace requires fork: use SysProcAttr.
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
        Unshareflags: syscall.CLONE_NEWNS,
    }

    // Start the process. When using CLONE_NEWPID the child is the namespace root; the actual command
    // will be executed as PID 1 under the new PID namespace when the shell runs.
    // Prepare cgroup if requested
    var cgPath string
    var cleanup func() error
    var err error
    if c.Resources != nil {
        name := fmt.Sprintf("cloudforge-%d", timeNowUnixNano())
        cgPath, cleanup, err = setupCgroup(name, c.Resources)
        if err != nil {
            // best-effort: continue without cgroup
            cgPath = ""
            cleanup = nil
        }
    }

    if err := cmd.Start(); err != nil {
        if cleanup != nil {
            _ = cleanup()
        }
        return fmt.Errorf("runtime: start child: %w", err)
    }

    // If cgroup was created, add child PID to cgroup.procs
    if cgPath != "" {
        pid := cmd.Process.Pid
        if err := addPidToCgroup(cgPath, pid); err != nil {
            // best-effort: log to stderr
            fmt.Fprintf(os.Stderr, "runtime: add pid to cgroup: %v\n", err)
        }
    }

    waitErr := cmd.Wait()
    if cleanup != nil {
        _ = cleanup()
    }
    return waitErr
}

func quote(s string) string {
    return strconv.Quote(s)
}

func joinQuoted(args []string) string {
    parts := make([]string, len(args))
    for i, a := range args {
        parts[i] = quote(a)
    }
    // use /bin/sh -c exec "arg0" "arg1" ... style
    return strings.Join(parts, " ")
}

// timeNowUnixNano is a small helper to avoid importing time in many places.
func timeNowUnixNano() int64 {
    return time.Now().UnixNano()
}

// setupCgroup creates a cgroup v2 directory under /sys/fs/cgroup and applies limits.
func setupCgroup(name string, r *Resources) (string, func() error, error) {
    base := "/sys/fs/cgroup"
    path := filepath.Join(base, name)
    if err := os.MkdirAll(path, 0o755); err != nil {
        return "", nil, fmt.Errorf("cgroup: create dir: %w", err)
    }

    // Apply limits best-effort.
    if r.MemoryLimitBytes > 0 {
        if err := os.WriteFile(filepath.Join(path, "memory.max"), []byte(strconv.FormatInt(r.MemoryLimitBytes, 10)), 0o644); err != nil {
            return path, func() error { _ = os.Remove(path); return nil }, fmt.Errorf("cgroup: set memory: %w", err)
        }
    }
    if r.PidsLimit > 0 {
        if err := os.WriteFile(filepath.Join(path, "pids.max"), []byte(strconv.Itoa(r.PidsLimit)), 0o644); err != nil {
            return path, func() error { _ = os.Remove(path); return nil }, fmt.Errorf("cgroup: set pids: %w", err)
        }
    }
    if r.CPUQuotaPercent > 0 {
        // cgroup v2 cpu.max expects "<max|quota period>". Use period=100000us.
        period := 100000
        quota := (r.CPUQuotaPercent * period) / 100
        if quota < 1 {
            quota = 1
        }
        val := fmt.Sprintf("%d %d", quota, period)
        if err := os.WriteFile(filepath.Join(path, "cpu.max"), []byte(val), 0o644); err != nil {
            return path, func() error { _ = os.Remove(path); return nil }, fmt.Errorf("cgroup: set cpu: %w", err)
        }
    }

    cleanup := func() error {
        // Attempt to remove cgroup directory. If processes remain, removal will fail; ignore errors.
        _ = os.Remove(filepath.Join(path, "cgroup.procs"))
        _ = os.Remove(path)
        return nil
    }
    return path, cleanup, nil
}

func addPidToCgroup(path string, pid int) error {
    procs := filepath.Join(path, "cgroup.procs")
    return os.WriteFile(procs, []byte(strconv.Itoa(pid)), 0o644)
}


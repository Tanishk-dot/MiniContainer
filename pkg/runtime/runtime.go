package runtime

// Package runtime provides a minimal container runtime API.

// Container describes a simple container invocation.
type Container struct {
    // Command to run inside the container (argv form).
    Cmd []string
    // Hostname to set inside the UTS namespace.
    Hostname string
    // MountProc controls whether /proc is mounted inside the container.
    MountProc bool
    // Resources specifies cgroup resource limits (optional).
    Resources *Resources
}

// New creates a configured Container.
func New(cmd []string) *Container {
    return &Container{Cmd: cmd}
}

// Resources are a minimal set of cgroup limits.
type Resources struct {
    // Pids limit (max number of processes), 0 means unlimited.
    PidsLimit int
    // Memory limit in bytes, 0 means unlimited.
    MemoryLimitBytes int64
    // CPU quota as percentage (1-100), 0 means unlimited.
    CPUQuotaPercent int
}

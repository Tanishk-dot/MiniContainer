package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "time"

    "cloudforge/internal/config"
    "cloudforge/pkg/build"
    "cloudforge/pkg/image"
    "cloudforge/pkg/layer"
    "cloudforge/pkg/runtime"
    "cloudforge/pkg/storage"
)

func usage() {
    fmt.Fprintf(os.Stderr, "Usage: cloudforge <command> [args]\n")
    fmt.Fprintf(os.Stderr, "Commands:\n")
    fmt.Fprintf(os.Stderr, "  build              Build image from Dockerfile\n")
    fmt.Fprintf(os.Stderr, "  images             List images\n")
    fmt.Fprintf(os.Stderr, "  run                Run container\n")
    fmt.Fprintf(os.Stderr, "  stop               Stop container\n")
    fmt.Fprintf(os.Stderr, "  rm                 Remove container\n")
    fmt.Fprintf(os.Stderr, "  deploy             Deploy service with replicas\n")
    fmt.Fprintf(os.Stderr, "  scale              Scale deployment replicas\n")
    fmt.Fprintf(os.Stderr, "  status             Show deployment status\n")
    fmt.Fprintf(os.Stderr, "  delete             Delete deployment\n")
    os.Exit(2)
}

func main() {
    if len(os.Args) < 2 {
        usage()
    }
    cmd := os.Args[1]
    // internal child-run helper
    if cmd == "child-run" {
        if len(os.Args) < 3 {
            fmt.Fprintf(os.Stderr, "child-run <state-file>\n")
            os.Exit(2)
        }
        stateFile := os.Args[2]
        childRun(stateFile)
        return
    }

    cfg, err := config.Default()
    if err != nil {
        fmt.Fprintf(os.Stderr, "config: %v\n", err)
        os.Exit(1)
    }

    switch cmd {
    case "build":
        fs := flag.NewFlagSet("build", flag.ExitOnError)
        file := fs.String("file", "build.json", "build steps JSON file")
        _ = fs.Parse(os.Args[2:])
        runBuild(cfg, *file)
    case "images":
        runImages(cfg)
    case "run":
        fs := flag.NewFlagSet("run", flag.ExitOnError)
        bg := fs.Bool("detach", false, "run detached (background)")
        mem := fs.Int64("memory", 0, "memory limit bytes")
        pids := fs.Int("pids", 0, "pids limit")
        cpu := fs.Int("cpu", 0, "cpu percent")
        _ = fs.Parse(os.Args[2:])
        if fs.NArg() == 0 {
            fmt.Fprintf(os.Stderr, "run: command required\n")
            os.Exit(2)
        }
        args := fs.Args()
        runRun(cfg, args, *bg, &runtime.Resources{PidsLimit: *pids, MemoryLimitBytes: *mem, CPUQuotaPercent: *cpu})
    case "stop":
        if len(os.Args) < 3 {
            fmt.Fprintf(os.Stderr, "stop <id>\n")
            os.Exit(2)
        }
        runStop(cfg, os.Args[2])
    case "rm":
        if len(os.Args) < 3 {
            fmt.Fprintf(os.Stderr, "rm <id>\n")
            os.Exit(2)
        }
        runRm(cfg, os.Args[2])
    case "deploy":
        deployScheduler(cfg, os.Args[2:])
    case "scale":
        scaleScheduler(cfg, os.Args[2:])
    case "status":
        statusScheduler(cfg, os.Args[2:])
    case "delete":
        deleteScheduler(cfg, os.Args[2:])
    default:
        usage()
    }
}

func runBuild(cfg *config.Config, file string) {
    // read steps JSON
    f, err := os.Open(file)
    if err != nil {
        fmt.Fprintf(os.Stderr, "open build file: %v\n", err)
        os.Exit(1)
    }
    defer f.Close()
    var steps []build.Step
    dec := json.NewDecoder(f)
    if err := dec.Decode(&steps); err != nil {
        fmt.Fprintf(os.Stderr, "decode steps: %v\n", err)
        os.Exit(1)
    }

    // create layer manager
    s, err := storage.NewEngine(cfg)
    if err != nil {
        fmt.Fprintf(os.Stderr, "storage: %v\n", err)
        os.Exit(1)
    }
    lm := layer.NewManager(s.Blobs, layer.NewFileLayerMetadataStore(cfg.LayerMetadataDir()))
    eng := build.NewEngine(lm)
    ctx := context.Background()
    res, err := eng.Build(ctx, steps)
    if err != nil {
        fmt.Fprintf(os.Stderr, "build: %v\n", err)
        os.Exit(1)
    }
    for i, r := range res {
        fmt.Printf("%d: %s (%d)\n", i, r.ID.String(), r.Size)
    }
}

func runImages(cfg *config.Config) {
    ie, err := image.NewEngine(cfg)
    if err != nil {
        fmt.Fprintf(os.Stderr, "image engine: %v\n", err)
        os.Exit(1)
    }
    list, err := ie.Images.List(context.Background())
    if err != nil {
        fmt.Fprintf(os.Stderr, "list images: %v\n", err)
        os.Exit(1)
    }
    for _, m := range list {
        fmt.Println(m.Digest.String())
    }
}

func runRun(cfg *config.Config, args []string, detach bool, resources *runtime.Resources) {
    id := strconv.FormatInt(time.Now().UnixNano(), 10)
    containersDir := filepath.Join(cfg.MetadataDir(), "containers")
    _ = os.MkdirAll(containersDir, 0o755)
    stateFile := filepath.Join(containersDir, id+".json")

    cont := runtime.New(args)
    cont.MountProc = true
    cont.Resources = resources

    // Serialize container config to a temp file for child
    tmpcfg := filepath.Join(containersDir, id+".cfg.json")
    b, _ := json.Marshal(cont)
    if err := os.WriteFile(tmpcfg, b, 0o644); err != nil {
        fmt.Fprintf(os.Stderr, "write cfg: %v\n", err)
        os.Exit(1)
    }

    if detach {
        // Spawn background child process: this binary with child-run <tmpcfg>
        cmd := exec.Command(os.Args[0], "child-run", tmpcfg)
        if err := cmd.Start(); err != nil {
            fmt.Fprintf(os.Stderr, "start child: %v\n", err)
            os.Exit(1)
        }
        // Record state
        state := map[string]interface{}{"pid": cmd.Process.Pid, "cmd": args, "cfg": tmpcfg, "created_at": time.Now().UTC()}
        sb, _ := json.Marshal(state)
        if err := os.WriteFile(stateFile, sb, 0o644); err != nil {
            fmt.Fprintf(os.Stderr, "write state: %v\n", err)
            os.Exit(1)
        }
        fmt.Printf("Started %s (pid %d)\n", id, cmd.Process.Pid)
        return
    }

    // Foreground: execute in this process
    // Use Run directly
    if err := cont.Run(context.Background()); err != nil {
        fmt.Fprintf(os.Stderr, "run: %v\n", err)
        os.Exit(1)
    }
}

func childRun(stateFile string) {
    data, err := os.ReadFile(stateFile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "child-run read cfg: %v\n", err)
        os.Exit(1)
    }
    var cont runtime.Container
    if err := json.Unmarshal(data, &cont); err != nil {
        fmt.Fprintf(os.Stderr, "child-run unmarshal: %v\n", err)
        os.Exit(1)
    }
    if err := cont.Run(context.Background()); err != nil {
        fmt.Fprintf(os.Stderr, "child-run run: %v\n", err)
        os.Exit(1)
    }
}

func runStop(cfg *config.Config, id string) {
    stateFile := filepath.Join(cfg.MetadataDir(), "containers", id+".json")
    data, err := os.ReadFile(stateFile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "read state: %v\n", err)
        os.Exit(1)
    }
    var st struct{ Pid int `json:"pid"` }
    if err := json.Unmarshal(data, &st); err != nil {
        fmt.Fprintf(os.Stderr, "parse state: %v\n", err)
        os.Exit(1)
    }
    if err := exec.Command("/bin/kill", "-TERM", strconv.Itoa(st.Pid)).Run(); err != nil {
        fmt.Fprintf(os.Stderr, "kill: %v\n", err)
        os.Exit(1)
    }
    fmt.Printf("sent SIGTERM to %d\n", st.Pid)
}

func runRm(cfg *config.Config, id string) {
    base := filepath.Join(cfg.MetadataDir(), "containers")
    stateFile := filepath.Join(base, id+".json")
    cfgFile := filepath.Join(base, id+".cfg.json")
    _ = os.Remove(stateFile)
    _ = os.Remove(cfgFile)
    fmt.Printf("removed %s\n", id)
}

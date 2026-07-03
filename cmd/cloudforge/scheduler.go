package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"cloudforge/internal/config"
	"cloudforge/pkg/runtime"
	"cloudforge/pkg/scheduler"
)

// deployScheduler handles deploy command
func deployScheduler(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	replicas := fs.Int("replicas", 1, "number of replicas")
	memory := fs.Int64("memory", 0, "memory limit bytes")
	cpu := fs.Int("cpu", 0, "cpu percent")
	pids := fs.Int("pids", 0, "pids limit")
	labels := fs.String("labels", "", "comma-separated labels (key=value)")

	fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "deploy <name> <image>\n")
		os.Exit(1)
	}

	name := fs.Args()[0]
	image := fs.Args()[1]

	// Create scheduler
	sched, err := scheduler.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create scheduler: %v\n", err)
		os.Exit(1)
	}

	// Parse labels
	labelMap := make(map[string]string)
	if *labels != "" {
		for _, label := range strings.Split(*labels, ",") {
			parts := strings.SplitN(label, "=", 2)
			if len(parts) == 2 {
				labelMap[parts[0]] = parts[1]
			}
		}
	}

	// Create resources
	resources := &runtime.Resources{
		MemoryLimitBytes: *memory,
		CPUQuotaPercent:  *cpu,
		PidsLimit:        *pids,
	}
	if *memory == 0 && *cpu == 0 && *pids == 0 {
		resources = nil
	}

	// Deploy
	deployment, err := sched.Deploy(context.Background(), name, image, *replicas, resources, labelMap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deploy failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Deployed %s\n", name)
	fmt.Printf("  ID:        %s\n", deployment.ID)
	fmt.Printf("  Image:     %s\n", deployment.Image)
	fmt.Printf("  Replicas:  %d\n", deployment.Replicas)
	fmt.Printf("  Running:   %d\n", deployment.Running)
	fmt.Printf("  Created:   %s\n", deployment.CreatedAt.Format("2006-01-02 15:04:05"))
	if len(labelMap) > 0 {
		fmt.Printf("  Labels:    %v\n", labelMap)
	}
}

// scaleScheduler handles scale command
func scaleScheduler(cfg *config.Config, args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "scale <deployment-id> <replicas>\n")
		os.Exit(1)
	}

	deploymentID := args[0]
	replicas, err := strconv.Atoi(args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid replicas: %v\n", err)
		os.Exit(1)
	}

	// Create scheduler
	sched, err := scheduler.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create scheduler: %v\n", err)
		os.Exit(1)
	}

	// Scale
	deployment, err := sched.Scale(context.Background(), deploymentID, replicas)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scale failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Scaled %s to %d replicas\n", deployment.Name, replicas)
	fmt.Printf("  ID:        %s\n", deployment.ID)
	fmt.Printf("  Running:   %d/%d\n", deployment.Running, deployment.Replicas)
	fmt.Printf("  Updated:   %s\n", deployment.UpdatedAt.Format("2006-01-02 15:04:05"))
}

// statusScheduler handles status command
func statusScheduler(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	deploymentID := fs.String("deployment", "", "specific deployment ID")
	containers := fs.Bool("containers", false, "show container details")

	fs.Parse(args)

	// Create scheduler
	sched, err := scheduler.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create scheduler: %v\n", err)
		os.Exit(1)
	}

	if *deploymentID != "" {
		// Show specific deployment
		deployment, err := sched.GetStatus(*deploymentID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "deployment not found: %v\n", err)
			os.Exit(1)
		}

		printDeployment(deployment)

		if *containers {
			containerList, _ := sched.ListContainers(*deploymentID)
			printContainers(containerList)
		}
	} else {
		// List all deployments
		deployments := sched.ListDeployments()
		if len(deployments) == 0 {
			fmt.Println("No deployments")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tID\tIMAGE\tREPLICAS\tRUNNING\tSTATUS")

		for _, d := range deployments {
			status := "healthy"
			if d.Running < d.Replicas {
				status = "degraded"
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\n",
				d.Name, d.ID[:12], d.Image, d.Replicas, d.Running, status)
		}

		w.Flush()
	}
}

// deleteScheduler handles delete command
func deleteScheduler(cfg *config.Config, args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "delete <deployment-id>\n")
		os.Exit(1)
	}

	deploymentID := args[0]

	// Create scheduler
	sched, err := scheduler.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create scheduler: %v\n", err)
		os.Exit(1)
	}

	// Get deployment name first
	deployment, err := sched.GetStatus(deploymentID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deployment not found: %v\n", err)
		os.Exit(1)
	}

	// Delete
	err = sched.Delete(context.Background(), deploymentID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "delete failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Deleted deployment %s\n", deployment.Name)
}

// Helper functions

func printDeployment(deployment *scheduler.Deployment) {
	fmt.Printf("Deployment: %s\n", deployment.Name)
	fmt.Printf("  ID:         %s\n", deployment.ID)
	fmt.Printf("  Image:      %s\n", deployment.Image)
	fmt.Printf("  Replicas:   %d desired, %d running\n", deployment.Replicas, deployment.Running)
	fmt.Printf("  Created:    %s\n", deployment.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Updated:    %s\n", deployment.UpdatedAt.Format("2006-01-02 15:04:05"))

	if deployment.Resources != nil {
		fmt.Printf("  Resources:\n")
		if deployment.Resources.MemoryLimitBytes > 0 {
			fmt.Printf("    Memory:   %d bytes\n", deployment.Resources.MemoryLimitBytes)
		}
		if deployment.Resources.CPUQuotaPercent > 0 {
			fmt.Printf("    CPU:      %d%%\n", deployment.Resources.CPUQuotaPercent)
		}
		if deployment.Resources.PidsLimit > 0 {
			fmt.Printf("    PIDs:     %d\n", deployment.Resources.PidsLimit)
		}
	}

	if len(deployment.Labels) > 0 {
		fmt.Printf("  Labels:\n")
		for k, v := range deployment.Labels {
			fmt.Printf("    %s: %s\n", k, v)
		}
	}
}

func printContainers(containers []*scheduler.ContainerState) {
	if len(containers) == 0 {
		fmt.Println("No containers")
		return
	}

	fmt.Println("\nContainers:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tREPLICA\tSTATE\tPID\tSTARTED\tSTOPPED")

	for _, c := range containers {
		stopped := "-"
		if c.StoppedAt != nil {
			stopped = c.StoppedAt.Format("15:04:05")
		}

		fmt.Fprintf(w, "%s\t%d\t%s\t%d\t%s\t%s\n",
			c.ID, c.Replica, c.State, c.Pid, c.StartedAt.Format("15:04:05"), stopped)
	}

	w.Flush()
}

// HTTP client wrappers for remote scheduler API

func callSchedulerAPI(method, endpoint string, body io.Reader) ([]byte, error) {
	url := fmt.Sprintf("http://localhost:5001%s", endpoint)

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}

func deploySchedulerAPI(args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "deploy <name> <image> [-replicas N]\n")
		os.Exit(1)
	}

	name := args[0]
	image := args[1]
	replicas := 1

	// Parse optional replicas
	for i := 2; i < len(args); i++ {
		if args[i] == "-replicas" && i+1 < len(args) {
			r, _ := strconv.Atoi(args[i+1])
			if r > 0 {
				replicas = r
			}
			i++
		}
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"name":     name,
		"image":    image,
		"replicas": replicas,
	})

	resp, err := callSchedulerAPI("POST", "/deployments", strings.NewReader(string(payload)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "deploy failed: %v\n", err)
		os.Exit(1)
	}

	var deployment scheduler.Deployment
	json.Unmarshal(resp, &deployment)

	fmt.Printf("✓ Deployed %s\n", name)
	fmt.Printf("  ID: %s\n", deployment.ID)
	fmt.Printf("  Replicas: %d\n", deployment.Replicas)
}

func scaleSchedulerAPI(args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "scale <deployment-id> <replicas>\n")
		os.Exit(1)
	}

	id := args[0]
	replicas, _ := strconv.Atoi(args[1])

	payload, _ := json.Marshal(map[string]int{"replicas": replicas})
	resp, err := callSchedulerAPI("POST", fmt.Sprintf("/deployments/%s/scale", id), strings.NewReader(string(payload)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "scale failed: %v\n", err)
		os.Exit(1)
	}

	var deployment scheduler.Deployment
	json.Unmarshal(resp, &deployment)

	fmt.Printf("✓ Scaled %s to %d replicas\n", deployment.Name, replicas)
}

func statusSchedulerAPI(args []string) {
	var resp []byte
	var err error

	if len(args) > 0 {
		// Specific deployment
		resp, err = callSchedulerAPI("GET", fmt.Sprintf("/deployments/%s", args[0]), nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
			os.Exit(1)
		}

		var deployment scheduler.Deployment
		json.Unmarshal(resp, &deployment)
		printDeployment(&deployment)
	} else {
		// All deployments
		resp, err = callSchedulerAPI("GET", "/deployments", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
			os.Exit(1)
		}

		var result map[string]interface{}
		json.Unmarshal(resp, &result)

		if deployments, ok := result["deployments"].([]interface{}); ok && len(deployments) > 0 {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tREPLICAS\tRUNNING")
			for _, d := range deployments {
				if dmap, ok := d.(map[string]interface{}); ok {
					name := dmap["name"]
					replicas := dmap["replicas"]
					running := dmap["running"]
					fmt.Fprintf(w, "%v\t%v\t%v\n", name, replicas, running)
				}
			}
			w.Flush()
		} else {
			fmt.Println("No deployments")
		}
	}
}

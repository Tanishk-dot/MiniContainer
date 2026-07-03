package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"cloudforge/pkg/hash"
)

// RegistryClient provides a command-line interface to a CloudForge registry
type RegistryClient struct {
	baseURL string
}

// NewRegistryClient creates a new registry client
func NewRegistryClient(baseURL string) *RegistryClient {
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "http://" + baseURL
	}
	return &RegistryClient{baseURL: baseURL}
}

// Push uploads a local file to the registry as a blob
func (c *RegistryClient) Push(repo, localPath string) error {
	// Read file
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Calculate digest
	digest := hash.FromBytes(data)

	// Upload blob
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", c.baseURL, repo, digest.String())
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push failed (status %d): %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✓ Pushed blob: %s\n", digest.String())
	fmt.Printf("  Repository: %s\n", repo)
	fmt.Printf("  File: %s\n", localPath)
	fmt.Printf("  Size: %d bytes\n", len(data))

	return nil
}

// Pull downloads a blob from the registry
func (c *RegistryClient) Pull(repo, digest, outputPath string) error {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", c.baseURL, repo, digest)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pull failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Write to file
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	size, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("✓ Pulled blob: %s\n", digest)
	fmt.Printf("  Repository: %s\n", repo)
	fmt.Printf("  Output: %s\n", outputPath)
	fmt.Printf("  Size: %d bytes\n", size)

	return nil
}

// CheckBlob checks if a blob exists in the registry
func (c *RegistryClient) CheckBlob(repo, digest string) error {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", c.baseURL, repo, digest)

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		size := resp.Header.Get("Content-Length")
		fmt.Printf("✓ Blob exists\n")
		fmt.Printf("  Repository: %s\n", repo)
		fmt.Printf("  Digest: %s\n", digest)
		fmt.Printf("  Size: %s bytes\n", size)
		return nil
	}

	if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("✗ Blob not found\n")
		fmt.Printf("  Repository: %s\n", repo)
		fmt.Printf("  Digest: %s\n", digest)
		return nil
	}

	return fmt.Errorf("check failed (status %d)", resp.StatusCode)
}

// PushManifest uploads a manifest to the registry
func (c *RegistryClient) PushManifest(repo, tag, manifestPath string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Validate it's valid JSON
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("invalid manifest JSON: %w", err)
	}

	url := fmt.Sprintf("%s/v2/%s/manifests/%s", c.baseURL, repo, tag)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push manifest failed (status %d): %s", resp.StatusCode, string(body))
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	fmt.Printf("✓ Pushed manifest: %s\n", tag)
	fmt.Printf("  Repository: %s\n", repo)
	fmt.Printf("  Digest: %s\n", digest)
	fmt.Printf("  File: %s\n", manifestPath)

	return nil
}

// PullManifest downloads a manifest from the registry
func (c *RegistryClient) PullManifest(repo, reference, outputPath string) error {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", c.baseURL, repo, reference)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pull manifest failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Write to file
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	// Format the JSON nicely
	var manifest map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &manifest); err != nil {
		out.Write(body)
	} else {
		formatted, _ := json.MarshalIndent(manifest, "", "  ")
		out.Write(formatted)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	fmt.Printf("✓ Pulled manifest: %s\n", reference)
	fmt.Printf("  Repository: %s\n", repo)
	fmt.Printf("  Digest: %s\n", digest)
	fmt.Printf("  Output: %s\n", outputPath)

	return nil
}

// ListTags retrieves all tags for a repository
func (c *RegistryClient) ListTags(repo string) error {
	url := fmt.Sprintf("%s/v2/%s/tags/list", c.baseURL, repo)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("list failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	tags, ok := result["tags"].([]interface{})
	if !ok || tags == nil {
		fmt.Printf("No tags in repository: %s\n", repo)
		return nil
	}

	fmt.Printf("Tags for %s:\n", repo)
	for _, tag := range tags {
		fmt.Printf("  - %v\n", tag)
	}

	return nil
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "push-blob":
		if len(os.Args) < 5 {
			fmt.Println("Usage: cloudforge-registry push-blob <registry-url> <repo> <local-file>")
			os.Exit(1)
		}
		client := NewRegistryClient(os.Args[2])
		if err := client.Push(os.Args[3], os.Args[4]); err != nil {
			log.Fatal(err)
		}

	case "pull-blob":
		if len(os.Args) < 6 {
			fmt.Println("Usage: cloudforge-registry pull-blob <registry-url> <repo> <digest> <output-file>")
			os.Exit(1)
		}
		client := NewRegistryClient(os.Args[2])
		if err := client.Pull(os.Args[3], os.Args[4], os.Args[5]); err != nil {
			log.Fatal(err)
		}

	case "check-blob":
		if len(os.Args) < 5 {
			fmt.Println("Usage: cloudforge-registry check-blob <registry-url> <repo> <digest>")
			os.Exit(1)
		}
		client := NewRegistryClient(os.Args[2])
		if err := client.CheckBlob(os.Args[3], os.Args[4]); err != nil {
			log.Fatal(err)
		}

	case "push-manifest":
		if len(os.Args) < 6 {
			fmt.Println("Usage: cloudforge-registry push-manifest <registry-url> <repo> <tag> <manifest-file>")
			os.Exit(1)
		}
		client := NewRegistryClient(os.Args[2])
		if err := client.PushManifest(os.Args[3], os.Args[4], os.Args[5]); err != nil {
			log.Fatal(err)
		}

	case "pull-manifest":
		if len(os.Args) < 6 {
			fmt.Println("Usage: cloudforge-registry pull-manifest <registry-url> <repo> <reference> <output-file>")
			os.Exit(1)
		}
		client := NewRegistryClient(os.Args[2])
		if err := client.PullManifest(os.Args[3], os.Args[4], os.Args[5]); err != nil {
			log.Fatal(err)
		}

	case "list-tags":
		if len(os.Args) < 4 {
			fmt.Println("Usage: cloudforge-registry list-tags <registry-url> <repo>")
			os.Exit(1)
		}
		client := NewRegistryClient(os.Args[2])
		if err := client.ListTags(os.Args[3]); err != nil {
			log.Fatal(err)
		}

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`CloudForge Registry CLI

Commands:
  push-blob <url> <repo> <file>           Push a blob to registry
  pull-blob <url> <repo> <digest> <out>   Pull a blob from registry
  check-blob <url> <repo> <digest>        Check if blob exists
  push-manifest <url> <repo> <tag> <file> Push a manifest with tag
  pull-manifest <url> <repo> <ref> <out>  Pull a manifest by tag or digest
  list-tags <url> <repo>                  List all tags in repository
  help                                     Show this help

Examples:
  cloudforge-registry push-blob localhost:5000 myapp image.tar.gz
  cloudforge-registry list-tags localhost:5000 myapp
  cloudforge-registry pull-manifest localhost:5000 myapp v1.0 manifest.json
`)
}

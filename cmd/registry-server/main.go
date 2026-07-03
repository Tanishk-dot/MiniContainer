package main

import (
	"flag"
	"log"

	"cloudforge/internal/config"
	"cloudforge/pkg/registry"
)

// registryserver is a simple HTTP registry server for CloudForge
func main() {
	addr := flag.String("addr", ":5000", "Registry server address (host:port)")
	flag.Parse()

	log.Printf("Initializing CloudForge Registry...")

	// Create configuration
	cfg := &config.Config{}
	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("Failed to initialize directories: %v", err)
	}

	log.Printf("Blob directory: %s", cfg.BlobsDir())
	log.Printf("Metadata directory: %s", cfg.ImageMetadataDir())

	// Create registry
	reg, err := registry.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create registry: %v", err)
	}

	log.Printf("Starting CloudForge Registry on %s", *addr)
	log.Printf("API endpoints available at http://localhost%s/v2/", *addr)
	log.Printf("")
	log.Printf("Example commands:")
	log.Printf("  curl -X GET http://localhost%s/v2/", *addr)
	log.Printf("  cloudforge-registry list-tags localhost%s myrepo", *addr)
	log.Printf("")

	if err := reg.Start(*addr); err != nil {
		log.Fatalf("Registry server error: %v", err)
	}
}

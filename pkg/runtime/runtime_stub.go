//go:build !linux
// +build !linux

package runtime

import (
	"context"
	"fmt"
)

// Run is a stub implementation for non-Linux systems.
// On Windows, macOS, and other non-Linux platforms, container functionality is not available.
func (c *Container) Run(ctx context.Context) error {
	return fmt.Errorf("runtime: container execution not supported on this platform (requires Linux)")
}

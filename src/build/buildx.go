package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Buildx wraps docker buildx commands.
type Buildx struct {
	Verbose bool
	Stdout  io.Writer
	Stderr  io.Writer
}

// NewBuildx creates a Buildx runner with default output writers.
func NewBuildx(verbose bool) *Buildx {
	return &Buildx{
		Verbose: verbose,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
}

// Build executes a single build step via docker buildx.
func (bx *Buildx) Build(ctx context.Context, step BuildStep) (*StepResult, error) {
	start := time.Now()
	result := &StepResult{
		Name: step.Name,
	}

	args := bx.buildArgs(step)

	if bx.Verbose {
		fmt.Fprintf(bx.Stderr, "exec: docker %s\n", strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = bx.Stdout
	cmd.Stderr = bx.Stderr

	if err := cmd.Run(); err != nil {
		result.Status = "failed"
		result.Duration = time.Since(start)
		result.Error = fmt.Errorf("docker buildx build failed: %w", err)
		return result, result.Error
	}

	result.Status = "success"
	result.Duration = time.Since(start)
	result.Images = step.Tags

	return result, nil
}

// buildArgs constructs the docker buildx build argument list.
func (bx *Buildx) buildArgs(step BuildStep) []string {
	args := []string{"buildx", "build"}

	// Dockerfile
	if step.Dockerfile != "" {
		args = append(args, "--file", step.Dockerfile)
	}

	// Target stage
	if step.Target != "" {
		args = append(args, "--target", step.Target)
	}

	// Platforms
	if len(step.Platforms) > 0 {
		args = append(args, "--platform", strings.Join(step.Platforms, ","))
	}

	// Build args
	for k, v := range step.BuildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", k, v))
	}

	// Tags
	for _, tag := range step.Tags {
		args = append(args, "--tag", tag)
	}

	// Output mode
	switch {
	case step.Push:
		args = append(args, "--push")
	case step.Load:
		args = append(args, "--load")
	case step.Output == OutputLocal:
		// Extract to local filesystem — handled separately
		args = append(args, "--output", "type=local,dest=.")
	}

	// Build context
	context := step.Context
	if context == "" {
		context = "."
	}
	args = append(args, context)

	return args
}

// EnsureBuilder checks that a buildx builder is available and creates one if needed.
func (bx *Buildx) EnsureBuilder(ctx context.Context) error {
	// Check if default builder exists
	cmd := exec.CommandContext(ctx, "docker", "buildx", "inspect")
	if err := cmd.Run(); err != nil {
		// Create a builder
		create := exec.CommandContext(ctx, "docker", "buildx", "create", "--use", "--name", "stagefreight")
		create.Stdout = bx.Stderr
		create.Stderr = bx.Stderr
		if createErr := create.Run(); createErr != nil {
			return fmt.Errorf("creating buildx builder: %w", createErr)
		}
	}
	return nil
}

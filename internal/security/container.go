// Package security provides container security hardening options
package security

import "strconv"

// ContainerSecurityOptions defines security settings for agent containers
type ContainerSecurityOptions struct {
	// DropCapabilities specifies Linux capabilities to drop
	DropCapabilities []string

	// AddCapabilities specifies Linux capabilities to add
	AddCapabilities []string

	// NoNewPrivileges prevents processes from gaining new privileges
	NoNewPrivileges bool

	// ReadOnlyRootFilesystem makes the root filesystem read-only
	ReadOnlyRootFilesystem bool

	// PidsLimit limits the number of processes in the container
	PidsLimit int

	// MemoryLimit sets the memory limit (e.g., "4g")
	MemoryLimit string

	// CPULimit sets the CPU limit (e.g., "2")
	CPULimit string

	// SecurityOpts additional security options
	SecurityOpts []string
}

// DefaultContainerSecurityOptions returns secure defaults for containers
func DefaultContainerSecurityOptions() *ContainerSecurityOptions {
	return &ContainerSecurityOptions{
		DropCapabilities: []string{"ALL"},
		AddCapabilities: []string{
			"DAC_OVERRIDE", // Needed for file operations
			"CHOWN",        // Needed for file ownership changes
		},
		NoNewPrivileges:        true,
		ReadOnlyRootFilesystem: false, // Would break package installations
		PidsLimit:              1000,
		MemoryLimit:            "4g",
		CPULimit:               "2",
		SecurityOpts:           []string{"no-new-privileges"},
	}
}

// ToDockerArgs converts security options to Docker command line arguments
func (o *ContainerSecurityOptions) ToDockerArgs() []string {
	var args []string

	// Drop capabilities
	if len(o.DropCapabilities) > 0 {
		for _, cap := range o.DropCapabilities {
			args = append(args, "--cap-drop="+cap)
		}
	}

	// Add capabilities
	if len(o.AddCapabilities) > 0 {
		for _, cap := range o.AddCapabilities {
			args = append(args, "--cap-add="+cap)
		}
	}

	// Security options
	for _, opt := range o.SecurityOpts {
		args = append(args, "--security-opt="+opt)
	}

	// Resource limits
	if o.PidsLimit > 0 {
		args = append(args, "--pids-limit="+strconv.Itoa(o.PidsLimit))
	}

	if o.MemoryLimit != "" {
		args = append(args, "--memory="+o.MemoryLimit)
	}

	if o.CPULimit != "" {
		args = append(args, "--cpus="+o.CPULimit)
	}

	if o.ReadOnlyRootFilesystem {
		args = append(args, "--read-only")
	}

	return args
}
// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadKernelVersion returns the kernel release the node is running, read through
// the host proc mount rather than the agent's own namespace, so it describes the
// node and not the DaemonSet pod.
func ReadKernelVersion(hostProcPath string) (string, error) {
	releasePath := filepath.Join(hostProcPath, "sys", "kernel", "osrelease")
	content, err := os.ReadFile(releasePath)
	if err != nil {
		return "", fmt.Errorf("failed to read kernel version from %s: %w", releasePath, err)
	}
	release := strings.TrimSpace(string(content))
	if release == "" {
		return "", fmt.Errorf("kernel version at %s is empty", releasePath)
	}
	return release, nil
}

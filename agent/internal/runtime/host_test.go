// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func writeKernelRelease(t *testing.T, procPath, content string) {
	t.Helper()
	dir := filepath.Join(procPath, "sys", "kernel")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "osrelease"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestReadKernelVersion(t *testing.T) {
	procPath := t.TempDir()
	writeKernelRelease(t, procPath, "5.15.0-119-generic\n")

	got, err := ReadKernelVersion(procPath)
	if err != nil {
		t.Fatalf("ReadKernelVersion: %v", err)
	}
	if got != "5.15.0-119-generic" {
		t.Fatalf("ReadKernelVersion() = %q, want 5.15.0-119-generic", got)
	}
}

// An unreadable or blank release is reported rather than passed on: a fact
// recorded as the empty string would be indistinguishable from one this agent
// version never recorded at all.
func TestReadKernelVersionRejectsWhatItCannotRead(t *testing.T) {
	if _, err := ReadKernelVersion(t.TempDir()); err == nil {
		t.Fatal("expected an error when the host proc mount has no osrelease")
	}

	procPath := t.TempDir()
	writeKernelRelease(t, procPath, "\n")
	if _, err := ReadKernelVersion(procPath); err == nil {
		t.Fatal("expected an error when osrelease is blank")
	}
}

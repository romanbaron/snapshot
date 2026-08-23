// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

import "testing"

func populatedFacts() Facts {
	return Facts{
		Host: HostFacts{
			KernelVersion: "6.8.0-45-generic",
			CPUArch:       "amd64",
			AgentVersion:  "v0.3.1",
		},
		Pod: PodFacts{
			Image:       "nvcr.io/nvidia/tritonserver:24.09",
			ImageID:     "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			CPULimit:    "8",
			MemoryLimit: "32Gi",
		},
		GPU: GPUFacts{
			DriverVersion: "580.82.07",
			Devices: []GPUDevice{
				{UUID: "GPU-1111", ProductName: "Tesla T4"},
			},
		},
		Mounts: MountFacts{
			Externalized: []string{"/model-cache"},
			Existing:     []string{"/model-cache"},
		},
	}
}

func differentFacts() Facts {
	return Facts{
		Host: HostFacts{
			KernelVersion: "5.15.0-89-generic",
			CPUArch:       "arm64",
			AgentVersion:  "v0.1.0",
		},
		Pod: PodFacts{
			Image:       "nvcr.io/nvidia/tritonserver:24.01",
			ImageID:     "sha256:2222222222222222222222222222222222222222222222222222222222222222",
			CPULimit:    "1",
			MemoryLimit: "1Gi",
		},
		GPU: GPUFacts{
			DriverVersion: "560.35.03",
			Devices: []GPUDevice{
				{UUID: "GPU-2222", ProductName: "NVIDIA A100-SXM4-40GB"},
			},
		},
		Mounts: MountFacts{
			Externalized: []string{"/model-cache"},
			Existing:     nil,
		},
	}
}

// An empty policy table refuses nothing, whatever it is handed. Every rule is
// registered by a later change, and each one brings its own cases here.
func TestCompareWithoutRegisteredChecks(t *testing.T) {
	tests := []struct {
		name   string
		source Facts
		target Facts
	}{
		{
			name: "both sides empty",
		},
		{
			name:   "source populated, target empty",
			source: populatedFacts(),
		},
		{
			name:   "target populated, source empty",
			target: populatedFacts(),
		},
		{
			name:   "both sides populated and equal",
			source: populatedFacts(),
			target: populatedFacts(),
		},
		{
			name:   "both sides populated and different",
			source: populatedFacts(),
			target: differentFacts(),
		},
	}

	for _, gate := range []Gate{GatePreflight, GateInspect} {
		for _, tc := range tests {
			t.Run(string(gate)+" "+tc.name, func(t *testing.T) {
				if mismatches := Compare(gate, tc.source, tc.target); len(mismatches) != 0 {
					t.Fatalf("Compare(%q) reported %v, want no mismatches", gate, mismatches)
				}
			})
		}
	}
}

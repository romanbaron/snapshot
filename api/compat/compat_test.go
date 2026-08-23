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

// Whatever rules are registered, a fact nobody recorded cannot refuse anything:
// every checkpoint captured before a fact existed has to stay restorable, and a
// target the agent could not read has to be given the benefit of the doubt.
func TestCompareIgnoresUnknownFacts(t *testing.T) {
	tests := []struct {
		name   string
		source Facts
		target Facts
	}{
		{
			name: "neither side knows anything",
		},
		{
			name:   "the checkpoint recorded facts the target cannot describe",
			source: populatedFacts(),
		},
		{
			name:   "the target describes facts the checkpoint never recorded",
			target: populatedFacts(),
		},
		{
			name:   "both sides agree",
			source: populatedFacts(),
			target: populatedFacts(),
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

// Every registered rule reports itself, or a refusal names an empty check and
// nobody can tell which rule turned the restore down.
func TestEveryCheckIsNamedAndGated(t *testing.T) {
	seen := map[Check]bool{}
	for _, c := range checks {
		if c.name == "" {
			t.Error("policy table holds a rule with no name")
		}
		if c.gate != GatePreflight && c.gate != GateInspect {
			t.Errorf("check %q runs at gate %q, which no gate calls", c.name, c.gate)
		}
		if seen[c.name] {
			t.Errorf("check %q is registered twice", c.name)
		}
		seen[c.name] = true
	}
}

// Compare has to attribute a mismatch to the rule that found it, since the whole
// refusal vocabulary is built on the check name.
func TestCompareNamesTheFailingCheck(t *testing.T) {
	mismatches := Compare(GatePreflight, populatedFacts(), differentFacts())

	if len(mismatches) == 0 {
		t.Fatal("Compare found nothing wrong between two entirely different machines")
	}
	for _, mismatch := range mismatches {
		if mismatch.Check == "" {
			t.Errorf("mismatch %+v does not name the check that reported it", mismatch)
		}
	}
}

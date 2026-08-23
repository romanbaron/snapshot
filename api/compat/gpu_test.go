// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

import (
	"reflect"
	"testing"
)

func gpus(models ...string) Facts {
	devices := make([]GPUDevice, 0, len(models))
	for i, model := range models {
		devices = append(devices, GPUDevice{UUID: "GPU-" + string(rune('a'+i)), ProductName: model})
	}
	return Facts{GPU: GPUFacts{Devices: devices}}
}

func TestGPUModelCheck(t *testing.T) {
	tests := []struct {
		name   string
		source Facts
		target Facts
		want   []Mismatch
	}{
		{
			name:   "same model",
			source: gpus("NVIDIA L4"),
			target: gpus("NVIDIA L4"),
		},
		{
			name:   "different model",
			source: gpus("NVIDIA L4"),
			target: gpus("NVIDIA A100-SXM4-40GB"),
			want: []Mismatch{{
				Check:  CheckGPUModel,
				Source: "NVIDIA L4 x1",
				Target: "NVIDIA A100-SXM4-40GB x1",
			}},
		},
		{
			// Which GPU is allocated at which index is the device map's concern.
			// For the model, two of the same is two of the same.
			name:   "the same models in another order",
			source: gpus("NVIDIA L4", "Tesla T4"),
			target: gpus("Tesla T4", "NVIDIA L4"),
		},
		{
			name:   "one model replaced in a mixed set",
			source: gpus("NVIDIA L4", "Tesla T4"),
			target: gpus("NVIDIA L4", "NVIDIA L4"),
			want: []Mismatch{{
				Check:  CheckGPUModel,
				Source: "NVIDIA L4 x1, Tesla T4 x1",
				Target: "NVIDIA L4 x2",
			}},
		},
		{
			name:   "checkpoint taken before the models were recorded",
			source: Facts{GPU: GPUFacts{Devices: []GPUDevice{{UUID: "GPU-a"}}}},
			target: gpus("NVIDIA L4"),
		},
		{
			// The GPUs were found but could not be described, which is not the
			// same as being different.
			name:   "target models could not be read",
			source: gpus("NVIDIA L4"),
			target: Facts{GPU: GPUFacts{Devices: []GPUDevice{{UUID: "GPU-a"}}}},
		},
		{
			name:   "no GPUs at all",
			source: gpus("NVIDIA L4"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Compare(GateInspect, tc.source, tc.target)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Compare = %+v, want %+v", got, tc.want)
			}
		})
	}

	// The GPUs a container can see are only readable once it exists, so the
	// first gate has nothing to compare.
	if got := Compare(GatePreflight, gpus("NVIDIA L4"), gpus("Tesla T4")); len(got) != 0 {
		t.Errorf("the first gate judged a GPU it cannot see: %+v", got)
	}
}

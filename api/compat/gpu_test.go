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
			// Surrounding whitespace is already insignificant to the blank
			// guard, and a refusal printing two identical-looking names is one
			// nobody can act on.
			name:   "the same model padded on one side",
			source: gpus("NVIDIA L4"),
			target: gpus(" NVIDIA L4 "),
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

func TestGPUCountCheck(t *testing.T) {
	tests := []struct {
		name   string
		source Facts
		target Facts
		want   []Mismatch
	}{
		{
			name:   "the same number of GPUs",
			source: gpus("NVIDIA L4", "NVIDIA L4"),
			target: gpus("NVIDIA L4", "NVIDIA L4"),
		},
		{
			// Both rules speak: the set of models differs by a count, and so
			// does the number of GPUs. Each says something the other does not,
			// and a refusal names every rule it failed.
			name:   "fewer GPUs than were captured",
			source: gpus("NVIDIA L4", "NVIDIA L4"),
			target: gpus("NVIDIA L4"),
			want: []Mismatch{
				{Check: CheckGPUModel, Source: "NVIDIA L4 x2", Target: "NVIDIA L4 x1"},
				{Check: CheckGPUCount, Source: "2", Target: "1"},
			},
		},
		{
			// More is not better here: a checkpoint records one piece of device
			// state per GPU, and a spare GPU has no rank to take.
			name:   "more GPUs than were captured",
			source: gpus("NVIDIA L4"),
			target: gpus("NVIDIA L4", "NVIDIA L4"),
			want: []Mismatch{
				{Check: CheckGPUModel, Source: "NVIDIA L4 x1", Target: "NVIDIA L4 x2"},
				{Check: CheckGPUCount, Source: "1", Target: "2"},
			},
		},
		{
			name:   "the checkpoint recorded no GPUs",
			target: gpus("NVIDIA L4"),
		},
		{
			name:   "no GPUs were discovered on the target",
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

	// A checkpoint from before the models were recorded still knows how many
	// GPUs it used, so the count keeps applying where the model rule cannot.
	unnamed := Facts{GPU: GPUFacts{Devices: []GPUDevice{{UUID: "GPU-a"}, {UUID: "GPU-b"}}}}
	want := []Mismatch{{Check: CheckGPUCount, Source: "2", Target: "1"}}
	if got := Compare(GateInspect, unnamed, gpus("NVIDIA L4")); !reflect.DeepEqual(got, want) {
		t.Errorf("Compare = %+v, want %+v", got, want)
	}
}

func TestDriverVersionCheck(t *testing.T) {
	driver := func(version string) Facts {
		return Facts{GPU: GPUFacts{DriverVersion: version}}
	}

	tests := []struct {
		name   string
		source Facts
		target Facts
		want   []Mismatch
	}{
		{
			name:   "the same driver build",
			source: driver("580.65.06"),
			target: driver("580.65.06"),
		},
		{
			// Two builds of the same release are not interchangeable: upstream
			// reproduces a restore failure across exactly this distance.
			name:   "a different build of the same release",
			source: driver("580.65.06"),
			target: driver("580.65.08"),
			want: []Mismatch{{
				Check:  CheckDriverVersion,
				Source: "580.65.06",
				Target: "580.65.08",
			}},
		},
		{
			name:   "a newer driver",
			source: driver("580.65.06"),
			target: driver("585.10.01"),
			want: []Mismatch{{
				Check:  CheckDriverVersion,
				Source: "580.65.06",
				Target: "585.10.01",
			}},
		},
		{
			// Both rules speak: a different driver, and one CUDA checkpoint and
			// restore is not supported on at all.
			name:   "a driver below the floor",
			source: driver("580.65.06"),
			target: driver("560.35.03"),
			want: []Mismatch{
				{Check: CheckDriverVersion, Source: "580.65.06", Target: "560.35.03"},
				{Check: CheckDriverMinimum, Source: "580 or newer", Target: "560.35.03"},
			},
		},
		{
			name:   "both sides below the floor",
			source: driver("560.35.03"),
			target: driver("560.35.03"),
			want: []Mismatch{
				{Check: CheckDriverMinimum, Source: "580 or newer", Target: "560.35.03"},
			},
		},
		{
			name:   "checkpoint taken before the driver was recorded",
			target: driver("580.65.06"),
		},
		{
			name:   "the target driver could not be read",
			source: driver("580.65.06"),
		},
		{
			// A version in a form the floor cannot read is unknown rather than
			// old, so it does not refuse every restore on the node.
			name:   "an unreadable version",
			source: driver("vendor-build"),
			target: driver("vendor-build"),
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
}

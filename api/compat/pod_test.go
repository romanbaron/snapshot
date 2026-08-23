// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

import (
	"reflect"
	"testing"
)

func TestImageCheck(t *testing.T) {
	image := func(reference string) Facts {
		return Facts{Pod: PodFacts{Image: reference}}
	}

	tests := []struct {
		name   string
		source Facts
		target Facts
		want   []Mismatch
	}{
		{
			name:   "same image",
			source: image("nvcr.io/nvidia/tritonserver:24.09-py3"),
			target: image("nvcr.io/nvidia/tritonserver:24.09-py3"),
		},
		{
			name:   "different tag",
			source: image("nvcr.io/nvidia/tritonserver:24.09-py3"),
			target: image("nvcr.io/nvidia/tritonserver:24.01-py3"),
			want: []Mismatch{{
				Check:  CheckImage,
				Source: "nvcr.io/nvidia/tritonserver:24.09-py3",
				Target: "nvcr.io/nvidia/tritonserver:24.01-py3",
			}},
		},
		{
			name:   "checkpoint taken before the image was recorded",
			target: image("nvcr.io/nvidia/tritonserver:24.09-py3"),
		},
		{
			name:   "target image unknown",
			source: image("nvcr.io/nvidia/tritonserver:24.09-py3"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Compare(GatePreflight, tc.source, tc.target)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Compare = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestImageDigestCheck(t *testing.T) {
	const (
		captured = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
		rebuilt  = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	)
	imageID := func(id string) Facts {
		return Facts{Pod: PodFacts{ImageID: id}}
	}

	tests := []struct {
		name   string
		source Facts
		target Facts
		want   []Mismatch
	}{
		{
			name:   "same content",
			source: imageID(captured),
			target: imageID(captured),
		},
		{
			// The same reference resolved to different content, which is what a
			// rebuilt or moved tag looks like from here.
			name:   "same reference, rebuilt content",
			source: imageID(captured),
			target: imageID(rebuilt),
			want:   []Mismatch{{Check: CheckImageDigest, Source: captured, Target: rebuilt}},
		},
		{
			// Runtimes wrap the digest differently, and the artifact keeps
			// whatever it was given. The same content must not read as a
			// mismatch because one side spells it out and the other does not.
			name:   "the same digest wrapped differently",
			source: imageID("docker-pullable://nvcr.io/nvidia/tritonserver@" + captured),
			target: imageID(captured),
		},
		{
			name:   "different content, wrapped differently",
			source: imageID("docker-pullable://nvcr.io/nvidia/tritonserver@" + captured),
			target: imageID(rebuilt),
			want:   []Mismatch{{Check: CheckImageDigest, Source: captured, Target: rebuilt}},
		},
		{
			name:   "checkpoint taken before the image ID was recorded",
			target: imageID(captured),
		},
		{
			// The kubelet has not published a status for the placeholder yet.
			name:   "target image ID not published yet",
			source: imageID(captured),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Compare(GatePreflight, tc.source, tc.target)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Compare = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestMemoryLimitCheck(t *testing.T) {
	memory := func(limit string) Facts {
		return Facts{Pod: PodFacts{MemoryLimit: limit}}
	}

	tests := []struct {
		name   string
		source Facts
		target Facts
		want   []Mismatch
	}{
		{
			name:   "the same limit",
			source: memory("32Gi"),
			target: memory("32Gi"),
		},
		{
			name:   "a larger limit",
			source: memory("32Gi"),
			target: memory("64Gi"),
		},
		{
			name:   "a smaller limit",
			source: memory("32Gi"),
			target: memory("1Gi"),
			want:   []Mismatch{{Check: CheckMemoryLimit, Source: "32Gi", Target: "1Gi"}},
		},
		{
			// The same amount written another way is the same amount.
			name:   "the same limit in different units",
			source: memory("32Gi"),
			target: memory("34359738368"),
		},
		{
			// A pod with no limit records none, so this is also how a restore
			// into an unlimited pod reads: nothing to compare.
			name:   "the target has no limit",
			source: memory("32Gi"),
		},
		{
			name:   "the checkpoint recorded no limit",
			target: memory("1Gi"),
		},
		{
			name:   "an unreadable quantity",
			source: memory("32Gi"),
			target: memory("plenty"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Compare(GatePreflight, tc.source, tc.target)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Compare = %+v, want %+v", got, tc.want)
			}
		})
	}
}

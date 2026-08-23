// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

import (
	"reflect"
	"testing"
)

func TestMountCheck(t *testing.T) {
	mounts := func(externalized, existing []string) Facts {
		return Facts{Mounts: MountFacts{Externalized: externalized, Existing: existing}}
	}

	tests := []struct {
		name  string
		facts Facts
		want  []Mismatch
	}{
		{
			name:  "every mount is there",
			facts: mounts([]string{"/model-cache", "/data"}, []string{"/model-cache", "/data"}),
		},
		{
			name:  "one mount is missing",
			facts: mounts([]string{"/model-cache", "/data"}, []string{"/model-cache"}),
			want:  []Mismatch{{Check: CheckMount, Source: "/data", Target: "missing"}},
		},
		{
			// Each missing volume is named, since a user fixing their pod needs
			// to know about all of them and not one at a time.
			name:  "the pod has none of them",
			facts: mounts([]string{"/model-cache", "/data"}, nil),
			want: []Mismatch{
				{Check: CheckMount, Source: "/model-cache", Target: "missing"},
				{Check: CheckMount, Source: "/data", Target: "missing"},
			},
		},
		{
			// CRIU reconstructs these itself, so their absence from the pod is
			// not a volume anybody forgot to declare.
			name:  "the mounts CRIU restores itself",
			facts: mounts([]string{"/", "/dev/shm", "/model-cache"}, []string{"/model-cache"}),
		},
		{
			name:  "the checkpoint externalized nothing",
			facts: mounts(nil, nil),
		},
		{
			// The target side is resolved from the recorded list, so a path the
			// pod has and the checkpoint never used is not the gate's business.
			name:  "the pod has more than the checkpoint used",
			facts: mounts([]string{"/model-cache"}, []string{"/model-cache", "/scratch"}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Compare(GateInspect, tc.facts, tc.facts)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Compare = %+v, want %+v", got, tc.want)
			}
		})
	}

	// Whether a path resolves is only knowable once the placeholder container
	// exists, which is after the first gate has already run.
	missing := mounts([]string{"/model-cache"}, nil)
	if got := Compare(GatePreflight, missing, missing); len(got) != 0 {
		t.Errorf("the first gate judged a mount it cannot see: %+v", got)
	}
}

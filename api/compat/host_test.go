// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

import (
	"reflect"
	"testing"
)

func TestCPUArchCheck(t *testing.T) {
	arch := func(value string) Facts {
		return Facts{Host: HostFacts{CPUArch: value}}
	}

	tests := []struct {
		name   string
		source Facts
		target Facts
		want   []Mismatch
	}{
		{
			name:   "same architecture",
			source: arch("amd64"),
			target: arch("amd64"),
		},
		{
			name:   "different architecture",
			source: arch("amd64"),
			target: arch("arm64"),
			want:   []Mismatch{{Check: CheckCPUArch, Source: "amd64", Target: "arm64"}},
		},
		{
			name:   "checkpoint taken before the architecture was recorded",
			target: arch("arm64"),
		},
		{
			name:   "target architecture unknown",
			source: arch("amd64"),
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

	// The architecture is decidable from the manifest and the node alone, so
	// waiting for the placeholder container would delay the refusal for nothing.
	if got := Compare(GateInspect, arch("amd64"), arch("arm64")); len(got) != 0 {
		t.Errorf("the second gate repeated the architecture check: %+v", got)
	}
}

func TestKernelVersionCheck(t *testing.T) {
	kernel := func(value string) Facts {
		return Facts{Host: HostFacts{KernelVersion: value}}
	}

	tests := []struct {
		name   string
		source Facts
		target Facts
		want   []Mismatch
	}{
		{
			name:   "same kernel release",
			source: kernel("5.15.0-1071-aws"),
			target: kernel("5.15.0-1071-aws"),
		},
		{
			// A kernel upgrade alone is enough to break a restore that worked,
			// so the same major.minor on a different release is not good enough.
			name:   "same major and minor, different release",
			source: kernel("5.15.0-1071-aws"),
			target: kernel("5.15.0-1082-aws"),
			want: []Mismatch{{
				Check:  CheckKernelVersion,
				Source: "5.15.0-1071-aws",
				Target: "5.15.0-1082-aws",
			}},
		},
		{
			name:   "checkpoint taken before the kernel was recorded",
			target: kernel("5.15.0-1071-aws"),
		},
		{
			name:   "target kernel unknown",
			source: kernel("5.15.0-1071-aws"),
		},
		{
			// Both rules fire: the node runs a different kernel, and one no
			// restore of a modern glibc can succeed on.
			name:   "target below the floor",
			source: kernel("5.15.0-1071-aws"),
			target: kernel("5.4.0-150-generic"),
			want: []Mismatch{
				{Check: CheckKernelVersion, Source: "5.15.0-1071-aws", Target: "5.4.0-150-generic"},
				{Check: CheckKernelMinimum, Source: "5.13 or newer", Target: "5.4.0-150-generic"},
			},
		},
		{
			name:   "both sides below the floor",
			source: kernel("4.19.0-25-amd64"),
			target: kernel("4.19.0-25-amd64"),
			want: []Mismatch{
				{Check: CheckKernelMinimum, Source: "5.13 or newer", Target: "4.19.0-25-amd64"},
			},
		},
		{
			name:   "exactly at the floor",
			source: kernel("5.13.0-52-generic"),
			target: kernel("5.13.0-52-generic"),
		},
		{
			name:   "a newer major is above the floor",
			source: kernel("6.8.0-45-generic"),
			target: kernel("6.8.0-45-generic"),
		},
		{
			// A release the floor cannot read is unknown rather than old, so a
			// kernel string in a form nobody anticipated does not refuse every
			// restore on the node.
			name:   "unreadable release",
			source: kernel("custom-build"),
			target: kernel("custom-build"),
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

func TestAgentVersionCheck(t *testing.T) {
	agent := func(value string) Facts {
		return Facts{Host: HostFacts{AgentVersion: value}}
	}

	tests := []struct {
		name   string
		source Facts
		target Facts
		want   []Mismatch
	}{
		{
			name:   "same release",
			source: agent("0.4.1"),
			target: agent("0.4.1"),
		},
		{
			// A patch release does not change the artifact layout, so it must
			// not invalidate a checkpoint that is otherwise restorable.
			name:   "same minor, different patch",
			source: agent("0.4.1"),
			target: agent("0.4.7"),
		},
		{
			name:   "different minor",
			source: agent("0.4.1"),
			target: agent("0.5.0"),
			want:   []Mismatch{{Check: CheckAgentVersion, Source: "0.4.1", Target: "0.5.0"}},
		},
		{
			name:   "different major",
			source: agent("v1.0.0"),
			target: agent("v2.0.0"),
			want:   []Mismatch{{Check: CheckAgentVersion, Source: "v1.0.0", Target: "v2.0.0"}},
		},
		{
			name:   "a leading v is not a difference",
			source: agent("v0.4.1"),
			target: agent("0.4.1"),
		},
		{
			// The rule this whole check exists for: a checkpoint from before the
			// agent recorded its version cannot be reasoned about at all.
			name:   "checkpoint written before the agent recorded its version",
			target: agent("0.4.1"),
			want:   []Mismatch{{Check: CheckAgentVersion, Target: "0.4.1"}},
		},
		{
			// CI installs the agent under an image tag that is not a release, so
			// this node cannot say which release it is - and must not turn every
			// restore on it down.
			name:   "target version is not a release",
			source: agent("0.4.1"),
			target: agent("abc1234-snapshot-agent"),
		},
		{
			// Including the case that would otherwise refuse: an unreadable
			// target version outranks an absent source version.
			name:   "target version is not a release and the source has none",
			target: agent("abc1234-snapshot-agent"),
		},
		{
			name:   "source version is not a release",
			source: agent("abc1234-snapshot-agent"),
			target: agent("0.4.1"),
		},
		{
			name: "neither side knows its version",
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

// A refusal has to say that the checkpoint records no agent version, since that
// is what tells an operator it was taken before the upgrade.
func TestAgentVersionRefusalNamesTheMissingVersion(t *testing.T) {
	mismatches := Compare(GatePreflight, Facts{}, Facts{Host: HostFacts{AgentVersion: "0.4.1"}})

	want := "agent-version: source unset, target 0.4.1"
	if got := Reasons(mismatches); got != want {
		t.Errorf("reason = %q, want %q", got, want)
	}
}

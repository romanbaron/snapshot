// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

import (
	"strconv"
	"strings"
)

// CheckCPUArch refuses a restore onto a different instruction set. A checkpoint
// holds register state, and CRIU has nowhere to put an x86 register file on an
// ARM core, so this one can never be waived by a bigger machine or a newer
// driver - it is the hardest of the rules.
const CheckCPUArch Check = "cpu-arch"

var cpuArchCheck = check{
	name:    CheckCPUArch,
	gate:    GatePreflight,
	compare: func(source, target Facts) []Mismatch { return mustMatch(source.Host.CPUArch, target.Host.CPUArch) },
}

// CheckKernelVersion refuses a restore onto a kernel other than the captured
// one. Restores that had worked for over a year have broken on a kernel upgrade
// alone: criu#2636.
const CheckKernelVersion Check = "kernel-version"

// CheckKernelMinimum refuses a kernel too old to restore a modern glibc at all.
// glibc uses rseq, which needs 5.13 (criu#2229), and glibc 2.35 and newer
// segfault on restore below it (criu#2552).
const CheckKernelMinimum Check = "kernel-minimum"

const (
	minKernelMajor = 5
	minKernelMinor = 13
)

var kernelVersionCheck = check{
	name: CheckKernelVersion,
	gate: GatePreflight,
	compare: func(source, target Facts) []Mismatch {
		return mustMatch(source.Host.KernelVersion, target.Host.KernelVersion)
	},
}

var kernelMinimumCheck = check{
	name: CheckKernelMinimum,
	gate: GatePreflight,
	compare: func(_, target Facts) []Mismatch {
		major, minor, ok := parseMajorMinor(target.Host.KernelVersion)
		if !ok || major > minKernelMajor || (major == minKernelMajor && minor >= minKernelMinor) {
			return nil
		}
		return []Mismatch{{
			Source: strconv.Itoa(minKernelMajor) + "." + strconv.Itoa(minKernelMinor) + " or newer",
			Target: target.Host.KernelVersion,
		}}
	},
}

// CheckAgentVersion refuses a checkpoint written by an agent of a different
// minor release. The artifact layout is the agent's private format, and only a
// patch release promises not to change it.
//
// This is the one rule where an unrecorded fact refuses: a checkpoint that does
// not say which agent wrote it predates the agent that records it, and nothing
// about its contents can be assumed.
const CheckAgentVersion Check = "agent-version"

var agentVersionCheck = check{
	name: CheckAgentVersion,
	gate: GatePreflight,
	compare: func(source, target Facts) []Mismatch {
		targetMajor, targetMinor, ok := parseMajorMinor(target.Host.AgentVersion)
		if !ok {
			// A version this node cannot read decides nothing: CI installs the
			// agent under image tags that are not releases at all, and refusing
			// every restore there would be worse than not checking.
			return nil
		}
		if source.Host.AgentVersion == "" {
			return []Mismatch{{Target: target.Host.AgentVersion}}
		}
		sourceMajor, sourceMinor, ok := parseMajorMinor(source.Host.AgentVersion)
		if !ok || (sourceMajor == targetMajor && sourceMinor == targetMinor) {
			return nil
		}
		return []Mismatch{{Source: source.Host.AgentVersion, Target: target.Host.AgentVersion}}
	},
}

// parseMajorMinor reads the leading major.minor of a version, ignoring whatever
// follows it: kernels carry a distro suffix as in "5.15.0-1071-aws", and agent
// versions an optional "v". A version it cannot read is unknown.
func parseMajorMinor(version string) (major, minor int, ok bool) {
	fields := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".", 3)
	if len(fields) < 2 {
		return 0, 0, false
	}
	major, majorOK := leadingNumber(fields[0])
	minor, minorOK := leadingNumber(fields[1])
	return major, minor, majorOK && minorOK
}

// leadingNumber reads the digits a version field starts with, ignoring whatever
// a distro appended to them.
func leadingNumber(field string) (int, bool) {
	end := 0
	for end < len(field) && field[end] >= '0' && field[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	value, err := strconv.Atoi(field[:end])
	return value, err == nil
}

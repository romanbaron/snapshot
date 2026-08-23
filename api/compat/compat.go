// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package compat compares what a checkpoint was captured on against what a
// restore target offers, so an incompatible restore is refused up front instead
// of failing deep inside CRIU with an unattributable error.
//
// It lives in the api module because that is the only module both the node agent
// (which enforces) and the operator (which publishes the recorded facts) already
// import, so the vocabulary cannot fork between them.
package compat

// Gate names the moment a comparison runs. The two gates see different facts:
// only the later one can read the node's GPUs and the target's rootfs.
type Gate string

const (
	// GatePreflight runs before the agent claims a restore attempt, where the
	// checkpoint manifest and the target pod are all that is readable.
	GatePreflight Gate = "preflight"

	// GateInspect runs once the placeholder container is resolved, where the
	// GPUs it sees and the mounts under its rootfs become readable.
	GateInspect Gate = "inspect"
)

// Check identifies one comparison rule. It is reported verbatim to users and to
// tooling that branches on it, so a name never changes once released.
type Check string

// Facts is one side of a comparison: the machine, pod, GPU and mount state a
// checkpoint was captured on, or the state a restore target offers.
//
// Every field is optional. A fact missing on either side is unknown rather than
// mismatched, because a checkpoint captured before that fact was ever recorded
// has to stay restorable.
type Facts struct {
	Host   HostFacts
	Pod    PodFacts
	GPU    GPUFacts
	Mounts MountFacts
}

// HostFacts describes the machine.
type HostFacts struct {
	KernelVersion string
	CPUArch       string
	AgentVersion  string
}

// PodFacts describes the container the checkpointed process ran in.
type PodFacts struct {
	Image       string
	ImageID     string
	CPULimit    string
	MemoryLimit string
}

// GPUFacts describes the GPUs the container could see.
type GPUFacts struct {
	DriverVersion string
	Devices       []GPUDevice
}

// GPUDevice is one visible GPU.
type GPUDevice struct {
	UUID        string
	ProductName string
}

// MountFacts describes the volumes CRIU externalized at capture, and which of
// them a machine actually has.
type MountFacts struct {
	// Externalized holds the mount destinations CRIU externalized at capture.
	Externalized []string

	// Existing holds the destinations that resolve on this machine. The agent
	// resolves them before comparing, so a comparison never touches disk.
	Existing []string
}

// Mismatch is one rule the target failed, carrying both compared values so the
// reported reason can name them.
type Mismatch struct {
	Check  Check
	Source string
	Target string
}

// check is one row of the policy table. compare returns nil when the rule passes
// or when a fact it needs is unknown, and may report more than one mismatch when
// a rule covers several values.
type check struct {
	name    Check
	gate    Gate
	compare func(source, target Facts) []Mismatch
}

// checks is the policy table: every compatibility rule, in the order they are
// reported, each pinned to the gate that can evaluate it.
var checks = []check{
	cpuArchCheck,
	kernelVersionCheck,
	kernelMinimumCheck,
	agentVersionCheck,
}

// mustMatch reports a mismatch unless the two values are identical. A value
// absent on either side is unknown, and an unknown fact never refuses a restore:
// a checkpoint captured before it was ever recorded has to stay restorable.
func mustMatch(source, target string) []Mismatch {
	if source == "" || target == "" || source == target {
		return nil
	}
	return []Mismatch{{Source: source, Target: target}}
}

// Compare reports every rule the target fails at the given gate. An empty result
// means the restore may proceed.
func Compare(gate Gate, source, target Facts) []Mismatch {
	var mismatches []Mismatch
	for _, c := range checks {
		if c.gate != gate {
			continue
		}
		for _, mismatch := range c.compare(source, target) {
			mismatch.Check = c.name
			mismatches = append(mismatches, mismatch)
		}
	}
	return mismatches
}

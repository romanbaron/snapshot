// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

// CheckMount refuses a restore into a pod that is missing a path the checkpoint
// had mounted. CRIU was told to leave those mounts alone and expect them to be
// there; where one is absent, the restored process gets a working directory or a
// dataset that simply is not there, and finds out by failing later.
const CheckMount Check = "mount"

// criuHandledMounts are recorded as externalized but reconstructed by CRIU
// itself, so their absence from the target pod is not a missing volume.
var criuHandledMounts = map[string]bool{
	"/":        true,
	"/dev/shm": true,
}

var mountCheck = check{
	name: CheckMount,
	gate: GateInspect,
	compare: func(source, target Facts) []Mismatch {
		existing := make(map[string]bool, len(target.Mounts.Existing))
		for _, path := range target.Mounts.Existing {
			existing[path] = true
		}

		var mismatches []Mismatch
		for _, path := range source.Mounts.Externalized {
			if existing[path] || criuHandledMounts[path] {
				continue
			}
			mismatches = append(mismatches, Mismatch{Source: path, Target: "missing"})
		}
		return mismatches
	},
}

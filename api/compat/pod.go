// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

// CheckImage refuses a restore into a different image. A checkpoint is a process
// tree of the binaries and libraries that image provided, and restoring it over
// another image gives those pages a different program to belong to.
const CheckImage Check = "image"

// CheckImageDigest refuses a restore into the same image reference resolved to
// different content, which is what a rebuilt or moved tag looks like.
const CheckImageDigest Check = "image-digest"

var imageCheck = check{
	name:    CheckImage,
	gate:    GatePreflight,
	compare: func(source, target Facts) []Mismatch { return mustMatch(source.Pod.Image, target.Pod.Image) },
}

var imageDigestCheck = check{
	name: CheckImageDigest,
	gate: GatePreflight,
	compare: func(source, target Facts) []Mismatch {
		return mustMatch(imageDigest(source.Pod.ImageID), imageDigest(target.Pod.ImageID))
	},
}

// CheckMemoryLimit refuses a restore into less memory than the checkpoint was
// captured with. Restoring faults the whole recorded address space back in, so a
// lower ceiling is not a slower restore but an OOM kill partway through one.
const CheckMemoryLimit Check = "memory-limit"

var memoryLimitCheck = check{
	name: CheckMemoryLimit,
	gate: GatePreflight,
	compare: func(source, target Facts) []Mismatch {
		return atLeastSource(source.Pod.MemoryLimit, target.Pod.MemoryLimit)
	},
}

// CheckCPULimit refuses a restore into less CPU than the checkpoint was captured
// with. Unlike memory, too little does not fail: the workload restores, reports
// success, and runs measurably slower forever, which is the worst outcome of the
// three because nothing says it happened.
const CheckCPULimit Check = "cpu-limit"

var cpuLimitCheck = check{
	name: CheckCPULimit,
	gate: GatePreflight,
	compare: func(source, target Facts) []Mismatch {
		return atLeastSource(source.Pod.CPULimit, target.Pod.CPULimit)
	},
}

// atLeastSource reports a mismatch when the target is given less than the
// checkpoint was captured with. A quantity absent or unreadable on either side is
// unknown - which is also how an unlimited pod reads, since a pod with no limit
// records none.
//
// It is deliberately blunt: a deployment that was genuinely over-provisioned and
// is being trimmed on purpose is refused too, and the escape hatch is the way
// out of that.
func atLeastSource(source, target string) []Mismatch {
	sourceQuantity, err := resource.ParseQuantity(source)
	if err != nil {
		return nil
	}
	targetQuantity, err := resource.ParseQuantity(target)
	if err != nil {
		return nil
	}
	if targetQuantity.Cmp(sourceQuantity) >= 0 {
		return nil
	}
	return []Mismatch{{Source: source, Target: target}}
}

// imageDigest reduces a container status image ID to the digest inside it.
// Runtimes disagree on the wrapping - containerd reports a bare "sha256:...",
// others a scheme and a repository around it - and the artifact keeps whichever
// form it was given, so the two are only comparable after this.
func imageDigest(imageID string) string {
	digest := strings.TrimSpace(imageID)
	if scheme := strings.Index(digest, "://"); scheme >= 0 {
		digest = digest[scheme+len("://"):]
	}
	if at := strings.LastIndex(digest, "@"); at >= 0 {
		digest = digest[at+1:]
	}
	return digest
}

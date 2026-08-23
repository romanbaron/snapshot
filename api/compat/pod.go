// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

import "strings"

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

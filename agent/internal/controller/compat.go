// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/go-logr/logr"

	"github.com/ai-dynamo/snapshot/agent/internal/types"
	"github.com/ai-dynamo/snapshot/api/compat"
)

// reportRefusal reports one refused restore. Both gates report through here, so
// a refusal reads the same whichever one turned it down.
func (w *NodeController) reportRefusal(log logr.Logger, mismatches []compat.Mismatch) {
	log.Info("Refusing restore; this node cannot run the checkpoint", "reason", compat.Reasons(mismatches))
}

// preflightMismatches runs the pre-flight compatibility gate for one restore.
// An empty result means the restore may be attempted.
func (w *NodeController) preflightMismatches(log logr.Logger, artifactPath string) []compat.Mismatch {
	manifest, err := types.ReadManifest(artifactPath)
	if err != nil {
		// An unreadable manifest is not an incompatibility. The restore path
		// reads it again and reports the real error from there, so refusing here
		// would relabel a broken artifact as an incompatible one.
		log.V(1).Info("Skipping restore compatibility gate; checkpoint manifest is unreadable",
			"checkpoint_location", artifactPath,
			"error", err.Error(),
		)
		return nil
	}

	// Target facts are read by the rules that need them, since each one costs a
	// syscall or an API read on a path that runs before every restore.
	return w.compareFn(compat.GatePreflight, manifest.CompatFacts(), compat.Facts{})
}

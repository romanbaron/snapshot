// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/ai-dynamo/snapshot/agent/internal/types"
	"github.com/ai-dynamo/snapshot/api/compat"
	snapshotv1alpha1 "github.com/ai-dynamo/snapshot/api/v1alpha1"
)

// refusedRestore is one restore a gate turned down, and everything needed to
// report it.
type refusedRestore struct {
	pod           *corev1.Pod
	containerName string
	containerID   string
	mismatches    []compat.Mismatch
}

// reportRefusal reports one refused restore. Both gates report through here, so
// a refusal reads the same whichever one turned it down.
func (w *NodeController) reportRefusal(ctx context.Context, log logr.Logger, refused refusedRestore) {
	reason := compat.Reasons(refused.mismatches)
	log.Info("Refusing restore; this node cannot run the checkpoint", "reason", reason)
	emitPodEvent(
		ctx, w.clientset, log, refused.pod, "snapshot",
		corev1.EventTypeWarning, restoreIncompatibleReason, reason,
	)

	annotations, err := snapshotv1alpha1.RestoreStatusAnnotations(
		refused.containerName,
		snapshotv1alpha1.RestoreStatusIncompatible,
		refused.containerID,
	)
	if err != nil {
		log.Error(err, "Cannot record the refusal on the pod")
		return
	}
	// The refusal stands whether or not it can be recorded. annotatePod logs the
	// failure, and the log line and the event above already carry the reason.
	_ = annotatePod(ctx, w.clientset, log, refused.pod, annotations)
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

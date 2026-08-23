// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"runtime"

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

	annotations, err := snapshotv1alpha1.RestoreIncompatibleAnnotations(
		refused.containerName,
		refused.containerID,
		reason,
	)
	if err != nil {
		log.Error(err, "Cannot record the refusal on the pod")
		return
	}
	// The refusal stands whether or not it can be recorded. annotatePod logs the
	// failure, and the log line and the event above already carry the reason.
	_ = annotatePod(ctx, w.clientset, log, refused.pod, annotations)
}

// podFacts reads what one container of a pod runs as and is allowed. It serves
// both sides of a comparison: what a capture records about the source pod, and
// what a restore target offers.
//
// A container that is not in the pod, or a status the kubelet has not published
// yet, leaves the facts it would have supplied unknown.
func podFacts(pod *corev1.Pod, containerName string) compat.PodFacts {
	if pod == nil {
		return compat.PodFacts{}
	}

	facts := compat.PodFacts{}
	for _, container := range pod.Spec.Containers {
		if container.Name != containerName {
			continue
		}
		facts.Image = container.Image
		facts.CPULimit = limitString(container.Resources.Limits, corev1.ResourceCPU)
		facts.MemoryLimit = limitString(container.Resources.Limits, corev1.ResourceMemory)
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == containerName {
			facts.ImageID = status.ImageID
		}
	}
	return facts
}

// limitString keeps an unset limit unset. A missing quantity formats as "0",
// which would otherwise read as a container limited to nothing.
func limitString(limits corev1.ResourceList, name corev1.ResourceName) string {
	quantity, ok := limits[name]
	if !ok {
		return ""
	}
	return quantity.String()
}

// preflightMismatches runs the pre-flight compatibility gate for one restore.
// An empty result means the restore may be attempted.
func (w *NodeController) preflightMismatches(
	log logr.Logger,
	pod *corev1.Pod,
	containerName string,
	artifactPath string,
) []compat.Mismatch {
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

	return w.compareFn(
		compat.GatePreflight,
		manifest.CompatFacts(),
		w.preflightTargetFacts(pod, containerName),
	)
}

// preflightTargetFacts describes what this node and this pod offer a restore, as
// far as it is knowable before the placeholder container exists. It is assembled
// per restore from facts the agent already holds, so the gate costs no syscalls
// and no API reads.
func (w *NodeController) preflightTargetFacts(pod *corev1.Pod, containerName string) compat.Facts {
	return compat.Facts{
		Host: compat.HostFacts{
			// The agent's own architecture, which is the node's: this binary
			// could not be running here otherwise.
			CPUArch:       runtime.GOARCH,
			KernelVersion: w.config.Host.KernelVersion,
			AgentVersion:  w.config.Host.AgentVersion,
		},
		Pod: podFacts(pod, containerName),
	}
}

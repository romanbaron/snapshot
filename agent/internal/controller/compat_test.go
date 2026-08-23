// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/ai-dynamo/snapshot/agent/internal/types"
	"github.com/ai-dynamo/snapshot/api/compat"
	snapshotv1alpha1 "github.com/ai-dynamo/snapshot/api/v1alpha1"
)

type comparisonCall struct {
	gate   compat.Gate
	source compat.Facts
	target compat.Facts
}

// comparisonSpy stands in for the policy table so a test can force a refusal
// while the table itself is still being filled in.
type comparisonSpy struct {
	mismatches []compat.Mismatch
	calls      []comparisonCall
}

func (s *comparisonSpy) compare(gate compat.Gate, source, target compat.Facts) []compat.Mismatch {
	s.calls = append(s.calls, comparisonCall{gate: gate, source: source, target: target})
	return s.mismatches
}

// writeTestArtifact creates the artifact directory a restore pod resolves to,
// optionally with a manifest in it.
func writeTestArtifact(t *testing.T, basePath, checkpointID string, manifest *types.CheckpointManifest) string {
	t.Helper()
	dir := filepath.Join(basePath, checkpointID, "versions", snapshotv1alpha1.DefaultCheckpointArtifactVersion)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create artifact dir: %v", err)
	}
	if manifest != nil {
		if err := types.WriteManifest(dir, manifest); err != nil {
			t.Fatalf("failed to write manifest: %v", err)
		}
	}
	return dir
}

// A checkpoint whose manifest cannot be read is not incompatible, it is broken.
// The restore path reads the manifest again and reports that; refusing here
// would report the wrong outcome and hide the real error.
func TestPreflightMismatchesAllowsUnreadableManifest(t *testing.T) {
	w := makeTestController(t)
	spy := &comparisonSpy{mismatches: []compat.Mismatch{{Check: "kernel-version"}}}
	w.compareFn = spy.compare
	dir := writeTestArtifact(t, w.config.Storage.BasePath, "abc123", nil)

	if mismatches := w.preflightMismatches(w.log, dir); len(mismatches) != 0 {
		t.Fatalf("preflightMismatches() = %v, want none", mismatches)
	}
	if len(spy.calls) != 0 {
		t.Fatalf("comparison ran without a manifest: %#v", spy.calls)
	}
}

func TestPreflightMismatchesComparesRecordedFacts(t *testing.T) {
	w := makeTestController(t)
	spy := &comparisonSpy{}
	w.compareFn = spy.compare
	dir := writeTestArtifact(t, w.config.Storage.BasePath, "abc123", &types.CheckpointManifest{
		CheckpointID: "abc123",
		CRIUDump: types.CRIUDumpManifest{
			ExtMnt: map[string]string{
				"/model-cache": "/model-cache",
				"/etc/hosts":   "/etc/hosts",
			},
		},
	})

	if mismatches := w.preflightMismatches(w.log, dir); len(mismatches) != 0 {
		t.Fatalf("preflightMismatches() = %v, want none", mismatches)
	}
	if len(spy.calls) != 1 {
		t.Fatalf("comparison ran %d times, want once", len(spy.calls))
	}
	if got := spy.calls[0].gate; got != compat.GatePreflight {
		t.Fatalf("gate = %q, want %q", got, compat.GatePreflight)
	}
	want := []string{"/etc/hosts", "/model-cache"}
	if got := spy.calls[0].source.Mounts.Externalized; !reflect.DeepEqual(got, want) {
		t.Fatalf("externalized mounts = %#v, want %#v", got, want)
	}
}

// The gate sits before the attempt is claimed, so a refusal leaves no in-flight
// entry and no restore worker behind.
func TestReconcileRestorePodRefusesBeforeClaimingAttempt(t *testing.T) {
	checkpointID := "abc123"
	pod := makePod(
		"test-pod",
		"default",
		testNodeName,
		corev1.PodRunning,
		false,
		map[string]string{snapshotv1alpha1.CheckpointIDLabel: checkpointID},
		nil,
	)
	w := makeTestController(t, pod)
	spy := &comparisonSpy{mismatches: []compat.Mismatch{{
		Check:  "memory-limit",
		Source: "32Gi",
		Target: "1Gi",
	}}}
	w.compareFn = spy.compare
	writeTestArtifact(t, w.config.Storage.BasePath, checkpointID, &types.CheckpointManifest{CheckpointID: checkpointID})

	w.reconcileRestorePod(context.Background(), pod)

	if len(spy.calls) != 1 {
		t.Fatalf("comparison ran %d times, want once", len(spy.calls))
	}
	if sawEventReason(w.clientset.(*fake.Clientset), "RestoreRequested") {
		t.Fatal("refused restore still announced RestoreRequested")
	}
	if len(w.inFlight) != 0 {
		t.Fatalf("refused restore claimed an attempt: %#v", w.inFlight)
	}
}

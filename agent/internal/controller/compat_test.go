// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/ai-dynamo/snapshot/agent/internal/executor"
	snapshotruntime "github.com/ai-dynamo/snapshot/agent/internal/runtime"
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

// A refusal that reaches the worker from the second gate is terminal: it is not
// a CRIU failure, so it neither reports one nor kills the placeholder, and the
// attempt stays held so the same container is not tried again.
func TestRunRestoreTreatsIncompatibleAsTerminal(t *testing.T) {
	r := newRefusal(t)
	rt := &fakeRuntime{}
	r.controller.runtime = rt
	sentinels := 0
	r.controller.writeControlSentinelFn = func(int, string) error {
		sentinels++
		return nil
	}
	r.controller.restoreFn = func(context.Context, snapshotruntime.Runtime, logr.Logger, executor.RestoreRequest, executor.RestoreMounter) (int, error) {
		return 0, executor.NewRestoreIncompatibleError([]compat.Mismatch{{
			Check:  "cpu-arch",
			Source: "amd64",
			Target: "arm64",
		}})
	}
	attemptKey := "default/test-pod/main/ctr-abc"
	r.controller.inFlight[attemptKey] = struct{}{}

	err := r.controller.runRestore(
		context.Background(), r.pod, "main", "ctr-abc", refusalCheckpointID, attemptKey, time.Time{},
	)

	if err != nil {
		t.Fatalf("refusal surfaced as a worker error: %v", err)
	}
	if len(r.events(t, restoreFailedReason)) != 0 {
		t.Fatal("refusal reported itself as a restore failure")
	}
	if sentinels != 0 {
		t.Fatalf("refusal wrote %d restore-complete sentinels, want 0", sentinels)
	}
	if len(rt.resolvedContainerIDs) != 0 {
		t.Fatalf("refusal reached the placeholder kill path: %v", rt.resolvedContainerIDs)
	}
	if _, held := r.controller.inFlight[attemptKey]; !held {
		t.Fatal("refusal released the attempt, so the same container can be retried")
	}
}

// The gate sits before the attempt is claimed, so a refusal leaves no in-flight
// entry and no restore worker behind.
func TestReconcileRestorePodRefusesBeforeClaimingAttempt(t *testing.T) {
	r := newRefusal(t, compat.Mismatch{Check: "memory-limit", Source: "32Gi", Target: "1Gi"})

	r.reconcile(t)

	if len(r.comparison.calls) != 1 {
		t.Fatalf("comparison ran %d times, want once", len(r.comparison.calls))
	}
	if len(r.events(t, "RestoreRequested")) != 0 {
		t.Fatal("refused restore still announced RestoreRequested")
	}
	if len(r.controller.inFlight) != 0 {
		t.Fatalf("refused restore claimed an attempt: %#v", r.controller.inFlight)
	}
}

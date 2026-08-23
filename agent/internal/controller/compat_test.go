// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

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

// The gate compares the checkpoint against this node, so the target side has to
// describe the node the agent is running on and not stay empty.
func TestPreflightMismatchesDescribesThisNode(t *testing.T) {
	w := makeTestController(t)
	w.config.Host.KernelVersion = "5.15.0-1071-aws"
	w.config.Host.AgentVersion = "0.4.1"
	spy := &comparisonSpy{}
	w.compareFn = spy.compare
	dir := writeTestArtifact(t, w.config.Storage.BasePath, "abc123", &types.CheckpointManifest{
		CheckpointID: "abc123",
	})

	if mismatches := w.preflightMismatches(w.log, dir); len(mismatches) != 0 {
		t.Fatalf("preflightMismatches() = %v, want none", mismatches)
	}
	if len(spy.calls) != 1 {
		t.Fatalf("comparison ran %d times, want once", len(spy.calls))
	}
	want := compat.HostFacts{
		CPUArch:       runtime.GOARCH,
		KernelVersion: "5.15.0-1071-aws",
		AgentVersion:  "0.4.1",
	}
	if got := spy.calls[0].target.Host; !reflect.DeepEqual(got, want) {
		t.Fatalf("target host facts = %#v, want %#v", got, want)
	}
}

// Both gates log the same sentence for the same refusal, so an operator greps
// one field and does not have to know which gate turned the restore down.
func TestRefusalIsLoggedWithTheSameReasonAtBothGates(t *testing.T) {
	mismatch := compat.Mismatch{Check: "memory-limit", Source: "32Gi", Target: "1Gi"}
	wantReason := "memory-limit: source 32Gi, target 1Gi"

	t.Run("preflight gate", func(t *testing.T) {
		r := newRefusal(t, mismatch)

		r.reconcile(t)

		reasons := r.logs.valuesFor("reason")
		if len(reasons) != 1 {
			t.Fatalf("logged %d reasons, want one: %v", len(reasons), reasons)
		}
		if reasons[0] != wantReason {
			t.Fatalf("logged reason = %q, want %q", reasons[0], wantReason)
		}
		// The gate logger carries these, so a reader can find the pod a refusal
		// belongs to without correlating on time.
		if got := r.logs.valuesFor("pod"); len(got) == 0 || got[0] != "default/test-pod" {
			t.Fatalf("logged pod = %v, want default/test-pod", got)
		}
		if got := r.logs.valuesFor("container"); len(got) == 0 || got[0] != "main" {
			t.Fatalf("logged container = %v, want main", got)
		}
	})

	t.Run("node gate", func(t *testing.T) {
		r := newRefusal(t)
		r.controller.restoreFn = func(context.Context, snapshotruntime.Runtime, logr.Logger, executor.RestoreRequest, executor.RestoreMounter) (int, error) {
			return 0, executor.NewRestoreIncompatibleError([]compat.Mismatch{mismatch})
		}

		err := r.controller.runRestore(
			context.Background(), r.pod, "main", "ctr-abc", refusalCheckpointID, "attempt", time.Time{}, false,
		)
		if err != nil {
			t.Fatalf("runRestore: %v", err)
		}

		reasons := r.logs.valuesFor("reason")
		if len(reasons) != 1 {
			t.Fatalf("logged %d reasons, want one: %v", len(reasons), reasons)
		}
		if reasons[0] != wantReason {
			t.Fatalf("logged reason = %q, want %q", reasons[0], wantReason)
		}
	})
}

// The refusal event is its own reason, so alerting that pages on restore
// failures does not fire on a restore that was never attempted.
func TestRefusalEmitsOneIncompatibleEventAtBothGates(t *testing.T) {
	mismatch := compat.Mismatch{Check: "cpu-arch", Source: "amd64", Target: "arm64"}
	wantMessage := "cpu-arch: source amd64, target arm64"

	assertOneEvent := func(t *testing.T, r *refusal) {
		t.Helper()
		events := r.events(t, restoreIncompatibleReason)
		if len(events) != 1 {
			t.Fatalf("emitted %d %s events, want one", len(events), restoreIncompatibleReason)
		}
		if events[0].Message != wantMessage {
			t.Fatalf("event message = %q, want %q", events[0].Message, wantMessage)
		}
		if events[0].Type != corev1.EventTypeWarning {
			t.Fatalf("event type = %q, want %q", events[0].Type, corev1.EventTypeWarning)
		}
		if len(r.events(t, restoreFailedReason)) != 0 {
			t.Fatalf("refusal also emitted a %s event", restoreFailedReason)
		}
	}

	t.Run("preflight gate", func(t *testing.T) {
		r := newRefusal(t, mismatch)

		r.reconcile(t)

		assertOneEvent(t, r)
		if len(r.events(t, "RestoreRequested")) != 0 {
			t.Fatal("refused restore still announced RestoreRequested")
		}
	})

	t.Run("node gate", func(t *testing.T) {
		r := newRefusal(t)
		r.controller.restoreFn = func(context.Context, snapshotruntime.Runtime, logr.Logger, executor.RestoreRequest, executor.RestoreMounter) (int, error) {
			return 0, executor.NewRestoreIncompatibleError([]compat.Mismatch{mismatch})
		}

		if err := r.controller.runRestore(
			context.Background(), r.pod, "main", "ctr-abc", refusalCheckpointID, "attempt", time.Time{}, false,
		); err != nil {
			t.Fatalf("runRestore: %v", err)
		}

		assertOneEvent(t, r)
	})
}

// The pod carries its own verdict, so a refusal survives the agent restarting
// and is visible to anything that reads pods rather than events.
func TestRefusalRecordsIncompatibleStatusAtBothGates(t *testing.T) {
	mismatch := compat.Mismatch{Check: "gpu-model", Source: "Tesla T4", Target: "NVIDIA A100-SXM4-40GB"}
	keys, err := snapshotv1alpha1.RestoreStatusAnnotationKeysFor("main")
	if err != nil {
		t.Fatal(err)
	}

	assertRecorded := func(t *testing.T, r *refusal, containerID string) {
		t.Helper()
		annotations := r.annotations(t)
		if got := annotations[keys.Status]; got != snapshotv1alpha1.RestoreStatusIncompatible {
			t.Fatalf("restore status = %q, want %q", got, snapshotv1alpha1.RestoreStatusIncompatible)
		}
		if got := annotations[keys.ContainerID]; got != containerID {
			t.Fatalf("restore container id = %q, want %q", got, containerID)
		}
		// The same sentence the log line and the event carry, so a reader who
		// starts from the pod does not get a different answer.
		wantReason := "gpu-model: source Tesla T4, target NVIDIA A100-SXM4-40GB"
		if got := annotations[keys.Reason]; got != wantReason {
			t.Fatalf("restore reason = %q, want %q", got, wantReason)
		}
		if reasons := r.logs.valuesFor("reason"); len(reasons) != 1 || reasons[0] != wantReason {
			t.Fatalf("logged reasons = %v, want exactly %q", reasons, wantReason)
		}
	}

	t.Run("preflight gate", func(t *testing.T) {
		r := newRefusal(t, mismatch)

		r.reconcile(t)

		assertRecorded(t, r, testContainerID)
	})

	t.Run("node gate", func(t *testing.T) {
		r := newRefusal(t)
		r.controller.restoreFn = func(context.Context, snapshotruntime.Runtime, logr.Logger, executor.RestoreRequest, executor.RestoreMounter) (int, error) {
			return 0, executor.NewRestoreIncompatibleError([]compat.Mismatch{mismatch})
		}

		if err := r.controller.runRestore(
			context.Background(), r.pod, "main", "ctr-abc", refusalCheckpointID, "attempt", time.Time{}, false,
		); err != nil {
			t.Fatalf("runRestore: %v", err)
		}

		// The worker stamps in_progress before the restore runs, so the refusal
		// has to replace it rather than leave a restore that never resolves.
		assertRecorded(t, r, "ctr-abc")
	})
}

// Once refused, a pod is not re-examined: not on the next resync, and not when
// the container restarts under a new ID. The completed and failed statuses do
// retry on a new container, so this asserts the difference on purpose.
func TestReconcileRestorePodSkipsAlreadyRefusedContainer(t *testing.T) {
	keys, err := snapshotv1alpha1.RestoreStatusAnnotationKeysFor("main")
	if err != nil {
		t.Fatal(err)
	}
	refusedEarlier := map[string]string{
		keys.Status:      snapshotv1alpha1.RestoreStatusIncompatible,
		keys.ContainerID: "a-container-that-has-since-restarted",
		keys.Reason:      "cpu-arch: source amd64, target arm64",
	}

	r := newRefusal(t, compat.Mismatch{Check: "cpu-arch", Source: "amd64", Target: "arm64"})
	for key, value := range refusedEarlier {
		r.pod.Annotations[key] = value
	}

	r.reconcile(t)

	if len(r.comparison.calls) != 0 {
		t.Fatalf("already refused restore was compared again: %#v", r.comparison.calls)
	}
	if len(r.events(t, restoreIncompatibleReason)) != 0 {
		t.Fatal("already refused restore emitted another event")
	}
	if len(r.controller.inFlight) != 0 {
		t.Fatalf("already refused restore claimed an attempt: %#v", r.controller.inFlight)
	}
}

// The escape hatches: with either one set, neither gate runs, so a checkpoint
// the policy table would turn down is still attempted.
func TestSkipCompatCheckTurnsOffTheGates(t *testing.T) {
	mismatch := compat.Mismatch{Check: "cpu-arch", Source: "amd64", Target: "arm64"}

	// Lets the restore start and end quickly, since the point here is only
	// whether the gate let it through.
	attempt := func(t *testing.T, r *refusal) <-chan struct{} {
		t.Helper()
		r.controller.restoreFn = func(context.Context, snapshotruntime.Runtime, logr.Logger, executor.RestoreRequest, executor.RestoreMounter) (int, error) {
			return 0, errors.New("test restore stopped")
		}
		return observeEventReason(r.clientset(t), "RestoreWorkerFailed")
	}

	t.Run("gate never runs", func(t *testing.T) {
		r := newRefusal(t, mismatch)
		r.pod.Annotations[snapshotv1alpha1.SkipCompatCheckAnnotation] = "true"
		finished := attempt(t, r)

		r.reconcile(t)
		waitForSignal(t, finished, "the restore worker to run")

		if len(r.comparison.calls) != 0 {
			t.Fatalf("skipped gate compared anyway: %#v", r.comparison.calls)
		}
		if len(r.events(t, restoreIncompatibleReason)) != 0 {
			t.Fatal("skipped gate refused the restore")
		}
	})

	// A node with the gate off skips every restore it handles, whether or not
	// the pod asked for it.
	t.Run("node config turns it off for an unannotated pod", func(t *testing.T) {
		r := newRefusal(t, mismatch)
		r.controller.config.Restore.SkipCompatCheck = true
		finished := attempt(t, r)

		r.reconcile(t)
		waitForSignal(t, finished, "the restore worker to run")

		if len(r.comparison.calls) != 0 {
			t.Fatalf("skipped gate compared anyway: %#v", r.comparison.calls)
		}
		if len(r.events(t, restoreIncompatibleReason)) != 0 {
			t.Fatal("skipped gate refused the restore")
		}
	})

	// Gate B is inside the executor, past the point where either switch can be
	// read again, so the decision travels with the request. Without it, a
	// skipped restore would still be refused a few steps later.
	t.Run("the decision travels to the second gate", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			set  func(*refusal)
			want bool
		}{
			{name: "checked", set: func(*refusal) {}},
			{
				name: "skipped by pod",
				set: func(r *refusal) {
					r.pod.Annotations[snapshotv1alpha1.SkipCompatCheckAnnotation] = "true"
				},
				want: true,
			},
			{
				name: "skipped by node",
				set:  func(r *refusal) { r.controller.config.Restore.SkipCompatCheck = true },
				want: true,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				r := newRefusal(t)
				tc.set(r)
				var requested executor.RestoreRequest
				r.controller.restoreFn = func(_ context.Context, _ snapshotruntime.Runtime, _ logr.Logger, req executor.RestoreRequest, _ executor.RestoreMounter) (int, error) {
					requested = req
					return 0, errors.New("test restore stopped")
				}
				finished := observeEventReason(r.clientset(t), "RestoreWorkerFailed")

				r.reconcile(t)
				waitForSignal(t, finished, "the restore worker to run")

				if requested.SkipCompatCheck != tc.want {
					t.Fatalf("request carried SkipCompatCheck = %v, want %v", requested.SkipCompatCheck, tc.want)
				}
			})
		}
	})

	// The node switch is read per restore, not once at startup, which is what
	// makes flipping the ConfigMap enough to be heard.
	t.Run("node config is re-read for every restore", func(t *testing.T) {
		r := newRefusal(t, mismatch)
		reads := 0
		r.controller.skipCompatCheckFn = func() bool {
			reads++
			return reads > 1
		}
		finished := attempt(t, r)

		r.reconcile(t)
		if len(r.comparison.calls) != 1 {
			t.Fatalf("gate did not run while the switch was off: %#v", r.comparison.calls)
		}

		r.reconcile(t)
		waitForSignal(t, finished, "the restore worker to run")

		if len(r.comparison.calls) != 1 {
			t.Fatalf("gate ran after the switch was flipped on: %#v", r.comparison.calls)
		}
		if reads != 2 {
			t.Fatalf("switch was read %d times across two restores, want 2", reads)
		}
	})

	// The annotation has to reach a pod the gate already turned down, or the
	// only way out of a wrong refusal is deleting annotations by hand.
	t.Run("reaches an already refused pod", func(t *testing.T) {
		keys, err := snapshotv1alpha1.RestoreStatusAnnotationKeysFor("main")
		if err != nil {
			t.Fatal(err)
		}
		r := newRefusal(t, mismatch)
		r.pod.Annotations[keys.Status] = snapshotv1alpha1.RestoreStatusIncompatible
		r.pod.Annotations[keys.Reason] = mismatch.Reason()
		r.pod.Annotations[snapshotv1alpha1.SkipCompatCheckAnnotation] = "true"
		finished := attempt(t, r)

		r.reconcile(t)
		waitForSignal(t, finished, "the restore worker to run")

		if len(r.comparison.calls) != 0 {
			t.Fatalf("skipped gate compared anyway: %#v", r.comparison.calls)
		}
	})
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
		context.Background(), r.pod, "main", "ctr-abc", refusalCheckpointID, attemptKey, time.Time{}, false,
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

// The facts recorded at capture describe one container, so a multi-container pod
// must not contribute another container's image or limits.
func TestPodFactsReadTheTargetContainer(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{
				Name:  "sidecar",
				Image: "busybox:1.36",
				Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				}},
			},
			{
				Name:  "main",
				Image: "nvcr.io/nvidia/tritonserver:24.09-py3",
				Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				}},
			},
		}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
			{Name: "sidecar", ImageID: "sha256:sidecar"},
			{Name: "main", ImageID: "docker-pullable://nvcr.io/nvidia/tritonserver@sha256:deadbeef"},
		}},
	}

	want := compat.PodFacts{
		Image:       "nvcr.io/nvidia/tritonserver:24.09-py3",
		ImageID:     "docker-pullable://nvcr.io/nvidia/tritonserver@sha256:deadbeef",
		CPULimit:    "4",
		MemoryLimit: "16Gi",
	}
	if got := podFacts(pod, "main"); !reflect.DeepEqual(got, want) {
		t.Errorf("podFacts = %#v, want %#v", got, want)
	}
}

// A fact the pod does not carry stays unknown. An unlimited container is not a
// container limited to zero, and a status the kubelet has not published yet is
// not an image ID of "".
func TestPodFactsLeaveWhatThePodDoesNotSayUnknown(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:  "main",
			Image: "busybox:1.36",
			Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("16Gi"),
			}},
		}}},
	}

	want := compat.PodFacts{Image: "busybox:1.36", MemoryLimit: "16Gi"}
	if got := podFacts(pod, "main"); !reflect.DeepEqual(got, want) {
		t.Errorf("podFacts = %#v, want %#v", got, want)
	}

	if got := podFacts(pod, "absent"); (got != compat.PodFacts{}) {
		t.Errorf("podFacts for a container not in the pod = %#v, want unknown", got)
	}
	if got := podFacts(nil, "main"); (got != compat.PodFacts{}) {
		t.Errorf("podFacts without a pod = %#v, want unknown", got)
	}
}

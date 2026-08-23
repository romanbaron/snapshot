// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/ai-dynamo/snapshot/agent/internal/types"
	"github.com/ai-dynamo/snapshot/api/compat"
	snapshotv1alpha1 "github.com/ai-dynamo/snapshot/api/v1alpha1"
)

const refusalCheckpointID = "abc123"

// refusal is a restore that the compatibility gate turns down. Every test about
// what a refusal reports starts from one of these, so none of them has to build
// the pod, the artifact, the manifest and the comparison for itself.
type refusal struct {
	controller *NodeController
	pod        *corev1.Pod
	logs       *logRecorder
	comparison *comparisonSpy
}

func newRefusal(t *testing.T, mismatches ...compat.Mismatch) *refusal {
	t.Helper()
	pod := makePod(
		"test-pod",
		"default",
		testNodeName,
		corev1.PodRunning,
		false,
		map[string]string{snapshotv1alpha1.CheckpointIDLabel: refusalCheckpointID},
		nil,
	)
	w := makeTestController(t, pod)
	logs := &logRecorder{}
	w.log = logr.New(&recordingSink{recorder: logs})
	comparison := &comparisonSpy{mismatches: mismatches}
	w.compareFn = comparison.compare
	writeTestArtifact(t, w.config.Storage.BasePath, refusalCheckpointID, &types.CheckpointManifest{
		CheckpointID: refusalCheckpointID,
	})

	return &refusal{controller: w, pod: pod, logs: logs, comparison: comparison}
}

// reconcile drives the restore the way the pod informer does.
func (r *refusal) reconcile(t *testing.T) {
	t.Helper()
	r.controller.reconcileRestorePod(context.Background(), r.pod)
}

func (r *refusal) clientset(t *testing.T) *fake.Clientset {
	t.Helper()
	clientset, ok := r.controller.clientset.(*fake.Clientset)
	if !ok {
		t.Fatalf("controller clientset is %T, want *fake.Clientset", r.controller.clientset)
	}
	return clientset
}

// events returns every event emitted under one reason, so a test can assert on
// how many there are and not only that there was one.
func (r *refusal) events(t *testing.T, reason string) []*corev1.Event {
	t.Helper()
	var events []*corev1.Event
	for _, action := range r.clientset(t).Actions() {
		create, ok := action.(clientgotesting.CreateAction)
		if !ok || create.GetResource().Resource != "events" {
			continue
		}
		event, ok := create.GetObject().(*corev1.Event)
		if ok && event.Reason == reason {
			events = append(events, event)
		}
	}
	return events
}

type logRecord struct {
	message       string
	keysAndValues []any
}

// logRecorder captures what the agent logged, so a test can assert on the field
// an operator greps for rather than on a formatted sentence.
type logRecorder struct {
	mu      sync.Mutex
	records []logRecord
}

func (r *logRecorder) add(message string, inherited, keysAndValues []any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	merged := make([]any, 0, len(inherited)+len(keysAndValues))
	merged = append(merged, inherited...)
	merged = append(merged, keysAndValues...)
	r.records = append(r.records, logRecord{message: message, keysAndValues: merged})
}

// valuesFor returns every value logged under a key, whether it was attached to
// the record or inherited from the logger.
func (r *logRecorder) valuesFor(key string) []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var values []any
	for _, record := range r.records {
		for i := 0; i+1 < len(record.keysAndValues); i += 2 {
			if name, ok := record.keysAndValues[i].(string); ok && name == key {
				values = append(values, record.keysAndValues[i+1])
			}
		}
	}
	return values
}

type recordingSink struct {
	recorder *logRecorder
	values   []any
}

var _ logr.LogSink = (*recordingSink)(nil)

func (s *recordingSink) Init(logr.RuntimeInfo) {}
func (s *recordingSink) Enabled(int) bool      { return true }

func (s *recordingSink) Info(_ int, message string, keysAndValues ...any) {
	s.recorder.add(message, s.values, keysAndValues)
}

func (s *recordingSink) Error(err error, message string, keysAndValues ...any) {
	s.recorder.add(message, s.values, append(keysAndValues, "error", err))
}

// WithValues has to accumulate: the gates log through a logger that already
// carries the pod and container, and a test asserting on those must still see
// them on the record.
func (s *recordingSink) WithValues(keysAndValues ...any) logr.LogSink {
	values := make([]any, 0, len(s.values)+len(keysAndValues))
	values = append(values, s.values...)
	values = append(values, keysAndValues...)
	return &recordingSink{recorder: s.recorder, values: values}
}

func (s *recordingSink) WithName(string) logr.LogSink { return s }

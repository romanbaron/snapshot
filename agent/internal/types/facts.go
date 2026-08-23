// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"sort"

	"github.com/ai-dynamo/snapshot/api/compat"
)

// CompatFacts maps the manifest onto the fact model the compatibility gates
// compare. Both gates read it from here, so the two cannot disagree about what
// the checkpoint recorded.
func (m *CheckpointManifest) CompatFacts() compat.Facts {
	return compat.Facts{
		Host: compat.HostFacts{
			KernelVersion: m.Host.KernelVersion,
			CPUArch:       m.Host.CPUArch,
			AgentVersion:  m.Host.AgentVersion,
		},
		Pod: compat.PodFacts{
			Image:       m.K8s.Image,
			ImageID:     m.K8s.ImageID,
			CPULimit:    m.K8s.CPULimit,
			MemoryLimit: m.K8s.MemoryLimit,
		},
		GPU: m.gpuFacts(),
		Mounts: compat.MountFacts{
			Externalized: m.externalizedMounts(),
		},
	}
}

// WithPodFacts records what the captured container ran as. It is the inverse of
// the pod half of CompatFacts, and sits next to it so the two field lists cannot
// drift apart.
func (m SourcePodManifest) WithPodFacts(facts compat.PodFacts) SourcePodManifest {
	m.Image = facts.Image
	m.ImageID = facts.ImageID
	m.CPULimit = facts.CPULimit
	m.MemoryLimit = facts.MemoryLimit
	return m
}

// gpuFacts prefers the described GPUs and falls back to the UUID list, so an
// artifact captured before the models were recorded still reports its GPU count.
func (m *CheckpointManifest) gpuFacts() compat.GPUFacts {
	facts := compat.GPUFacts{DriverVersion: m.CUDA.SourceDriverVersion}
	if len(m.CUDA.SourceGPUs) > 0 {
		for _, gpu := range m.CUDA.SourceGPUs {
			facts.Devices = append(facts.Devices, compat.GPUDevice{
				UUID:        gpu.UUID,
				ProductName: gpu.ProductName,
			})
		}
		return facts
	}
	for _, uuid := range m.CUDA.SourceGPUUUIDs {
		facts.Devices = append(facts.Devices, compat.GPUDevice{UUID: uuid})
	}
	return facts
}

// externalizedMounts returns the destinations CRIU externalized at capture, in a
// stable order so a refusal always names them the same way.
func (m *CheckpointManifest) externalizedMounts() []string {
	if len(m.CRIUDump.ExtMnt) == 0 {
		return nil
	}
	destinations := make([]string, 0, len(m.CRIUDump.ExtMnt))
	for destination := range m.CRIUDump.ExtMnt {
		destinations = append(destinations, destination)
	}
	sort.Strings(destinations)
	return destinations
}

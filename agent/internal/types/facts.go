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
		GPU: m.gpuFacts(),
		Mounts: compat.MountFacts{
			Externalized: m.externalizedMounts(),
		},
	}
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

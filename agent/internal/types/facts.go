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
		Mounts: compat.MountFacts{
			Externalized: m.externalizedMounts(),
		},
	}
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

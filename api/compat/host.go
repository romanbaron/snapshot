// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

// CheckCPUArch refuses a restore onto a different instruction set. A checkpoint
// holds register state, and CRIU has nowhere to put an x86 register file on an
// ARM core, so this one can never be waived by a bigger machine or a newer
// driver - it is the hardest of the rules.
const CheckCPUArch Check = "cpu-arch"

var cpuArchCheck = check{
	name:    CheckCPUArch,
	gate:    GatePreflight,
	compare: func(source, target Facts) []Mismatch { return mustMatch(source.Host.CPUArch, target.Host.CPUArch) },
}

// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

import (
	"sort"
	"strconv"
	"strings"
)

// CheckGPUModel refuses a restore onto a different GPU model. A CUDA checkpoint
// carries device state built for one architecture's memory layout and
// capabilities, and no amount of driver compatibility makes an A100 replay what
// an L4 was doing.
const CheckGPUModel Check = "gpu-model"

var gpuModelCheck = check{
	name: CheckGPUModel,
	gate: GateInspect,
	compare: func(source, target Facts) []Mismatch {
		sourceModels, sourceOK := gpuModels(source.GPU)
		targetModels, targetOK := gpuModels(target.GPU)
		if !sourceOK || !targetOK || sourceModels == targetModels {
			return nil
		}
		return []Mismatch{{Source: sourceModels, Target: targetModels}}
	},
}

// CheckGPUCount refuses a restore onto a different number of GPUs. A multi-GPU
// checkpoint holds one piece of device state per GPU with a rank each, and there
// is no meaning to be given to a rank that has nowhere to land - or to a GPU no
// rank was recorded for.
const CheckGPUCount Check = "gpu-count"

var gpuCountCheck = check{
	name: CheckGPUCount,
	gate: GateInspect,
	compare: func(source, target Facts) []Mismatch {
		sourceCount := len(source.GPU.Devices)
		targetCount := len(target.GPU.Devices)
		if sourceCount == 0 || targetCount == 0 || sourceCount == targetCount {
			return nil
		}
		return []Mismatch{{
			Source: strconv.Itoa(sourceCount),
			Target: strconv.Itoa(targetCount),
		}}
	},
}

// CheckDriverVersion refuses a restore on a driver build other than the captured
// one. Build granularity is not caution for its own sake: upstream reproduces a
// restore failure between 560.35.03 and 560.35.05.
const CheckDriverVersion Check = "driver-version"

// CheckDriverMinimum refuses a driver older than CUDA checkpoint and restore is
// supported on at all.
const CheckDriverMinimum Check = "driver-minimum"

const minDriverMajor = 580

var driverVersionCheck = check{
	name: CheckDriverVersion,
	gate: GateInspect,
	compare: func(source, target Facts) []Mismatch {
		return mustMatch(source.GPU.DriverVersion, target.GPU.DriverVersion)
	},
}

var driverMinimumCheck = check{
	name: CheckDriverMinimum,
	gate: GateInspect,
	compare: func(_, target Facts) []Mismatch {
		major, ok := leadingNumber(target.GPU.DriverVersion)
		if !ok || major >= minDriverMajor {
			return nil
		}
		return []Mismatch{{
			Source: strconv.Itoa(minDriverMajor) + " or newer",
			Target: target.GPU.DriverVersion,
		}}
	},
}

// gpuModels renders the models one side offers as a counted, sorted list: the
// same GPUs allocated in another order are the same set, and which device sits
// at which index is the device map's problem rather than this rule's.
//
// A side is unknown unless every device is named, since comparing a partial list
// against a full one would refuse a restore over a name nobody could read.
func gpuModels(facts GPUFacts) (string, bool) {
	if len(facts.Devices) == 0 {
		return "", false
	}
	counts := make(map[string]int, len(facts.Devices))
	for _, device := range facts.Devices {
		if strings.TrimSpace(device.ProductName) == "" {
			return "", false
		}
		counts[device.ProductName]++
	}

	models := make([]string, 0, len(counts))
	for model := range counts {
		models = append(models, model)
	}
	sort.Strings(models)
	for i, model := range models {
		models[i] = model + " x" + strconv.Itoa(counts[model])
	}
	return strings.Join(models, ", "), true
}

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

// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package compat

import (
	"fmt"
	"strings"
)

// unsetValue stands in for a fact that was absent on one side, so a reason never
// renders as a dangling comma the reader has to interpret.
const unsetValue = "unset"

// reasonSeparator joins the reasons of a refusal that failed several rules.
const reasonSeparator = "; "

// Reason renders a mismatch as the sentence a user reads. The log field, the pod
// event and the pod annotation all carry this exact string, so an operator can
// match on one and find the others.
func (m Mismatch) Reason() string {
	return fmt.Sprintf("%s: source %s, target %s", m.Check, orUnset(m.Source), orUnset(m.Target))
}

// Reasons renders every mismatch of one refusal in report order.
func Reasons(mismatches []Mismatch) string {
	if len(mismatches) == 0 {
		return ""
	}
	reasons := make([]string, 0, len(mismatches))
	for _, mismatch := range mismatches {
		reasons = append(reasons, mismatch.Reason())
	}
	return strings.Join(reasons, reasonSeparator)
}

func orUnset(value string) string {
	if strings.TrimSpace(value) == "" {
		return unsetValue
	}
	return value
}

// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func validAgentConfig() *AgentConfig {
	return &AgentConfig{
		Storage: StorageSpec{
			Type:     "pvc",
			BasePath: "/checkpoints",
		},
		Restore: RestoreSpec{
			RestoreTimeoutSeconds: 60,
		},
	}
}

// The key an admin flips in the ConfigMap has to be the key the agent reads,
// and an agent whose ConfigMap predates it has to keep the gate on.
func TestRestoreSpecParsesSkipCompatCheck(t *testing.T) {
	cases := map[string]bool{
		"restore:\n  skipCompatCheck: true\n":     true,
		"restore:\n  skipCompatCheck: false\n":    false,
		"restore:\n  restoreTimeoutSeconds: 60\n": false,
	}
	for document, want := range cases {
		cfg := &AgentConfig{}
		if err := yaml.Unmarshal([]byte(document), cfg); err != nil {
			t.Fatalf("unmarshal %q: %v", document, err)
		}
		if cfg.Restore.SkipCompatCheck != want {
			t.Errorf("%q parsed skipCompatCheck = %v, want %v", document, cfg.Restore.SkipCompatCheck, want)
		}
	}
}

// The agent has no build stamp of its own, so its version can only come from
// the environment the chart sets.
func TestLoadEnvOverridesReadsTheAgentVersion(t *testing.T) {
	t.Run("recorded when set", func(t *testing.T) {
		t.Setenv("SNAPSHOT_AGENT_VERSION", " 0.4.1 ")
		cfg := &AgentConfig{}
		cfg.LoadEnvOverrides()
		if cfg.Host.AgentVersion != "0.4.1" {
			t.Fatalf("Host.AgentVersion = %q, want 0.4.1", cfg.Host.AgentVersion)
		}
	})

	// An agent that does not know its own version records nothing, rather than
	// a blank that later reads as a version.
	t.Run("left unknown when unset or blank", func(t *testing.T) {
		for _, value := range []string{"", "   "} {
			t.Setenv("SNAPSHOT_AGENT_VERSION", value)
			cfg := &AgentConfig{}
			cfg.LoadEnvOverrides()
			if cfg.Host.AgentVersion != "" {
				t.Fatalf("Host.AgentVersion = %q for %q, want empty", cfg.Host.AgentVersion, value)
			}
		}
	})
}

func TestAgentConfigValidateRequiresFixedStorageBasePath(t *testing.T) {
	for _, basePath := range []string{"checkpoints", " /checkpoints ", "/checkpoints/../other", "/other"} {
		cfg := validAgentConfig()
		cfg.Storage.BasePath = basePath
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate accepted storage base path %q", basePath)
		}
	}
}

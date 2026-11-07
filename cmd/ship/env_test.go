// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"os"
	"testing"
)

// TestMain gives every git command of these tests an identity through the
// environment (GIT_CONFIG_COUNT, git 2.31 and later), as CI runners have no
// global user.name or user.email, and turns off git's automatic maintenance:
// after a commit, git may start it in the background, and its lock files
// (.git/objects/maintenance.lock) come and go while a test compares the
// repository before and after a command. The developer's own git
// configuration is never written.
func TestMain(m *testing.M) {
	for name, value := range map[string]string{
		"GIT_CONFIG_COUNT":   "4",
		"GIT_CONFIG_KEY_0":   "user.name",
		"GIT_CONFIG_VALUE_0": "ship-it test",
		"GIT_CONFIG_KEY_1":   "user.email",
		"GIT_CONFIG_VALUE_1": "test@ship-it.invalid",
		"GIT_CONFIG_KEY_2":   "maintenance.auto",
		"GIT_CONFIG_VALUE_2": "false",
		"GIT_CONFIG_KEY_3":   "gc.auto",
		"GIT_CONFIG_VALUE_3": "0",
	} {
		if _, set := os.LookupEnv(name); !set {
			os.Setenv(name, value)
		}
	}
	os.Exit(m.Run())
}

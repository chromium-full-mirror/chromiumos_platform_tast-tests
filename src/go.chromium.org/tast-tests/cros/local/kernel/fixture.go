// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kernel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast/core/testing"
)

const (
	schedRRTimeslicePath = "/proc/sys/kernel/sched_rr_timeslice_ms"
	schedFairServerPath  = "/sys/kernel/debug/sched/fair_server"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.SchedRT,
		Desc:            "Fixture for setting Realtime scheduler",
		Contacts:        []string{"cros-sw-perf@google.com", "hsinyi@google.com"},
		Impl:            &schedRTFixture{},
		SetUpTimeout:    chrome.ManagedUserLoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.SchedRTGpuWatchHangs,
		Desc:            "Fixture for gpuWatchHangs and setting Realtime schedule",
		Contacts:        []string{"cros-sw-perf@google.com", "hsinyi@google.com"},
		Impl:            &schedRTFixture{},
		Parent:          "gpuWatchHangs", // Provides enrollment.
		SetUpTimeout:    chrome.ManagedUserLoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PostTestTimeout: 15 * time.Second,
	})
}

type schedRTFixture struct{}

type scheRTValues struct {
	SchedRRTimeslice  string            `json:"sched_rr_timeslice_ms"`
	FairServerRuntime map[string]string `json:"runtime"`
	FairServerPeriod  map[string]string `json:"period"`
}

func (i *schedRTFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Store old value
	schedRRTimeslice, err := os.ReadFile(schedRRTimeslicePath)
	if err != nil {
		s.Fatal("Failed to read sched_rr_timeslice_ms: ", err)
	}

	fairServerRuntime := make(map[string]string)
	fairServerPeriod := make(map[string]string)
	fairServers, err := filepath.Glob(schedFairServerPath + "/cpu*")
	if err != nil {
		s.Fatal("Failed to read fair_server: ", err)
	}

	for _, fairServer := range fairServers {
		runtimePath := fairServer + "/runtime"
		runtime, err := os.ReadFile(runtimePath)
		if err != nil {
			s.Fatal("Failed to read fair_server runtime: ", err)
		}
		fairServerRuntime[runtimePath] = strings.TrimRight(string(runtime), "\n")

		periodPath := fairServer + "/period"
		period, err := os.ReadFile(periodPath)
		if err != nil {
			s.Fatal("Failed to read fair_server period: ", err)
		}
		fairServerPeriod[periodPath] = strings.TrimRight(string(period), "\n")
	}

	oldSched := scheRTValues{
		SchedRRTimeslice:  strings.TrimRight(string(schedRRTimeslice), "\n"),
		FairServerRuntime: fairServerRuntime,
		FairServerPeriod:  fairServerPeriod,
	}

	jsonData, err := json.MarshalIndent(oldSched, "", "\t")
	if err != nil {
		s.Fatal("Failed to marshal json: ", err)
	}

	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Fatal("Failed to get OutDir from context")
	}
	if err := os.WriteFile(filepath.Join(dir, "old_sched"), jsonData,
		0644); err != nil {
		testing.ContextLog(ctx, "Failed to record old sched values: ", err)
	}

	// Write testing value
	if err := os.WriteFile(schedRRTimeslicePath, []byte("3"), 0644); err != nil {
		s.Fatal("Failed to reset sched_rr_timeslice_ms: ", err)
	}

	for _, fairServer := range fairServers {
		periodPath := fairServer + "/period"
		if err := os.WriteFile(periodPath, []byte("50000000"), 0644); err != nil {
			s.Fatal("Failed to reset fair_server period: ", err)
		}

		runtimePath := fairServer + "/runtime"
		if err := os.WriteFile(runtimePath, []byte("25000000"), 0644); err != nil {
			s.Fatal("Failed to reset fair_server runtime: ", err)
		}
	}

	return nil
}

func (i *schedRTFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	// Restore back the default value.
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Fatal("Failed to get OutDir from context")
	}
	oldSchedpath := filepath.Join(dir, "old_sched")
	jsonData, err := os.ReadFile(oldSchedpath)
	if err != nil {
		s.Fatal("Failed to read old sched setting: ", err)
	}

	var oldSched scheRTValues
	err = json.Unmarshal(jsonData, &oldSched)
	if err != nil {
		s.Fatal("Failed to parse old sched setting: ", err)
	}

	if err := os.WriteFile(schedRRTimeslicePath, []byte(oldSched.SchedRRTimeslice), 0644); err != nil {
		s.Fatal("Failed to reset sched_rr_timeslice_ms: ", err)
	}

	for path, value := range oldSched.FairServerPeriod {
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			s.Fatal("Failed to reset fair_server period: ", err)
		}
	}

	for path, value := range oldSched.FairServerRuntime {
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			s.Fatal("Failed to reset fair_server runtime: ", err)
		}
	}

	defer os.Remove(oldSchedpath)
}

func (i *schedRTFixture) Reset(ctx context.Context) error {
	return nil
}

func (i *schedRTFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}

func (i *schedRTFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}

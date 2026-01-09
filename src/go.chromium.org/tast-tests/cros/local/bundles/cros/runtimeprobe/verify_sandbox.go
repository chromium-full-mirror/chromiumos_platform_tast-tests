// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package runtimeprobe contains local tast tests that verifies runtime_probe.
package runtimeprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/debugd"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type verifySandboxTestParams struct {
	probeConfig   probeConfig
	helperTimeout int
}

func init() {
	testing.AddTest(&testing.Test{
		Func: VerifySandbox,
		Desc: "Checks runtime_probe sandbox",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"chungsheng@google.com",
		},
		BugComponent: "b:606088",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"racc"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{}}},
		}, {
			Name: "edid",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"edid"},
				}}},
		}, {
			Name: "input_device",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"input_device"},
				}}},
		}, {
			Name: "memory_amd64",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"memory"},
				}}},
			// This probe function can only be run on amd64.
			ExtraSoftwareDeps: []string{"amd64"},
		}, {
			Name: "battery",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"generic_battery"},
				}}},
		}, {
			Name: "camera",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"generic_camera"},
				}}},
		}, {
			Name: "network",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"network"},
				}}},
		}, {
			Name: "storage",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"generic_storage"},
				}}},
		}, {
			Name: "gpu",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"gpu"},
				}}},
		}, {
			Name: "audio_codec",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"audio_codec"},
				}}},
		}, {
			Name: "mmc_host",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"mmc_host"},
				}}},
		}, {
			Name: "generic_cpu",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"generic_cpu"},
				}}},
		}, {
			Name: "ec_component",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"ec_component"},
				}},
				helperTimeout: 15,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(
				// ec_component doesn't work properly in sandbox on the following models (see b/434803079).
				"deku", "kaladin", "kanix", "karis", "pujjolo", "uldrenite", "uldrenite360", "uldrino",
			)),
		}, {
			Name: "tpm",
			Val: verifySandboxTestParams{
				probeConfig: probeConfig{[]probeStatement{
					probeStatement{"tpm"},
				}}},
			ExtraAttr: []string{"informational"},
		}},
	})
}

// VerifySandbox verifies runtime_probe can work with sandbox.
func VerifySandbox(ctx context.Context, s *testing.State) {
	testParam := s.Param().(verifySandboxTestParams)
	pc := testParam.probeConfig
	helperTimeout := testParam.helperTimeout

	// Debugd is used by runtime_probe. Check we can connect to it to make the
	// error more specific.
	if _, err := debugd.New(ctx); err != nil {
		s.Fatal("Failed to connect to debugd: ", err)
	}

	res, err := getRuntimeProbeResult(ctx, pc, helperTimeout)
	if err != nil {
		s.Fatal("Failed to run runtime_probe: ", err)
	}
	resFactory, err := getFactoryRuntimeProbeResult(ctx, pc)
	if err != nil {
		s.Fatal("Failed to run factory_runtime_probe: ", err)
	}
	if diff := cmp.Diff(string(res), string(resFactory)); diff != "" {
		s.Fatalf("Result from runtime_probe and factory_runtime_probe mismatch (-want +got): %s", diff)
	}
	if !json.Valid(res) {
		s.Fatalf("Result from both runtime_probe are not valid json: %s", res)
	}
}

type probeConfig struct {
	probeStatements []probeStatement
}

type probeStatement struct {
	probeFunctionName string
}

func (pc probeConfig) MarshalJSON() ([]byte, error) {
	r := make(map[string]map[string]probeStatement)
	for _, ps := range pc.probeStatements {
		comp := make(map[string]probeStatement)
		comp[ps.probeFunctionName+"_name"] = ps
		r[ps.probeFunctionName+"_category"] = comp
	}
	return json.Marshal(r)
}

func (ps probeStatement) MarshalJSON() ([]byte, error) {
	pf := make(map[string]map[string]string)
	pf[ps.probeFunctionName] = make(map[string]string)
	r := make(map[string]map[string]map[string]string)
	r["eval"] = pf
	return json.Marshal(r)
}

func getRuntimeProbeResult(ctx context.Context, pc probeConfig, helperTimeout int) ([]byte, error) {
	f, err := os.CreateTemp("", "")
	if err != nil {
		return nil, errors.Wrap(err, "cannot create tmp file for runtime probe")
	}
	defer os.Remove(f.Name())

	pcj, err := json.Marshal(pc)
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal probe config")
	}

	if _, err := f.Write(pcj); err != nil {
		return nil, errors.Wrap(err, "cannot write probe config")
	}
	if err := f.Close(); err != nil {
		return nil, errors.Wrap(err, "cannot close probe config")
	}

	args := []string{"--config_file_path=" + f.Name(), "--to_stdout"}
	if helperTimeout > 0 {
		args = append(args, fmt.Sprintf("--helper_timeout=%d", helperTimeout))
	}
	cmd := testexec.CommandContext(ctx, "runtime_probe", args...)
	return cmd.Output(testexec.DumpLogOnError)
}

func getFactoryRuntimeProbeResult(ctx context.Context, pc probeConfig) ([]byte, error) {
	pcj, err := json.Marshal(pc)
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal probe config")
	}

	cmd := testexec.CommandContext(ctx, "factory_runtime_probe", string(pcj))
	return cmd.Output(testexec.DumpLogOnError)
}

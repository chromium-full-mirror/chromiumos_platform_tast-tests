// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package runtimeprobe contains local tast tests that verifies runtime_probe.
package runtimeprobe

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"

	"go.chromium.org/tast/core/testing"
)

type stringSet map[string]struct{}

func (set *stringSet) Contains(s string) bool {
	_, exists := (*set)[s]
	return exists
}

func (set *stringSet) Add(s string) {
	(*set)[s] = struct{}{}
}

type probeResult struct {
	Name   string            `json:"name"`
	Values map[string]string `json:"values"`
}

type audioProbeResults struct {
	AudioCodec []probeResult `json:"audio_codec"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AudioCodecCoverage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks audio codec kernel names coverage",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"clarkchung@google.com",
		},
		BugComponent: "b:606088",
		Attr:         []string{"group:racc", "racc_general"},
		SoftwareDeps: []string{"racc"},
		VarDeps: []string{
			"runtimeprobe.AudioCodecCoverage.allowlist",
			"runtimeprobe.AudioCodecCoverage.blocklist",
		},
		Timeout: 1 * time.Minute,
	})
}

// probeAudioCodec uses factory_runtime_probe to get the kernel names of audio
// codec components in DUT.
func probeAudioCodec(ctx context.Context) ([]string, error) {
	probeConfig := `{
  "audio_codec": {
    "generic": {
      "eval": {
        "audio_codec": {}
      }
    }
  }
}`
	output, err := testexec.CommandContext(ctx, "factory_runtime_probe", probeConfig).Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, err
	}
	var kernelNames []string
	var probeResults audioProbeResults
	if err := json.Unmarshal([]byte(output), &probeResults); err != nil {
		return nil, err
	}
	for _, probed := range probeResults.AudioCodec {
		if kernelName, ok := probed.Values["name"]; ok {
			kernelNames = append(kernelNames, kernelName)
		}
	}
	return kernelNames, nil
}

// AudioCodecCoverage checks the coverage of the curated allowlist and
// blocklist.  The probed result of the audio codec components should appear
// either in the allowlist or the blocklist.
func AudioCodecCoverage(ctx context.Context, s *testing.State) {
	allowlistJSON := s.RequiredVar("runtimeprobe.AudioCodecCoverage.allowlist")
	blocklistJSON := s.RequiredVar("runtimeprobe.AudioCodecCoverage.blocklist")
	var allowlist, blocklist []string
	json.Unmarshal([]byte(allowlistJSON), &allowlist)
	json.Unmarshal([]byte(blocklistJSON), &blocklist)
	knownKernelNames := &stringSet{}
	for _, kernelName := range allowlist {
		knownKernelNames.Add(kernelName)
	}
	for _, kernelName := range blocklist {
		knownKernelNames.Add(kernelName)
	}
	probedKernelNames, err := probeAudioCodec(ctx)
	if err != nil {
		s.Fatal("Cannot probe audio codec kernel names: ", err)
	}
	var unlisted []string
	for _, kernelName := range probedKernelNames {
		if !knownKernelNames.Contains(kernelName) {
			unlisted = append(unlisted, kernelName)
		}
	}
	sort.Strings(unlisted)
	if len(unlisted) > 0 {
		s.Errorf("Some kernel names do not exist in allowlist or blocklist: %q", unlisted)
	}
}

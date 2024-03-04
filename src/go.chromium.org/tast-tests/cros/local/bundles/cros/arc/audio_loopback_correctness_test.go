// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"strconv"
	"strings"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
)

func TestAudioLoopbackCorrectnessParams(t *testing.T) {
	const (
		classTestOutputSine = `"org.chromium.arc.testapp.arcaudiotest.TestOutputSineActivity"`

		performanceModeNone        = `arcaudio.PerformanceModeNone`
		performanceModeLowLatency  = `arcaudio.PerformanceModeLowLatency`
		performanceModePowerSaving = `arcaudio.PerformanceModePowerSaving`

		channelConfigOutStereo  = `arcaudio.ChannelConfigOutStereo`
		channelConfigOutQuad    = `arcaudio.ChannelConfigOutQuad`
		channelConfigOut5Point1 = `arcaudio.ChannelConfigOut5Point1`
	)
	type valMember struct {
		Key   string
		Value string
	}
	type val struct {
		ArcaudioTestParams   []valMember
		IncorrectSlicesLimit int
	}
	type paramData struct {
		Name              string
		Val               val
		ExtraHardwareDeps string
		ExtraAttr         string
		Fixture           string
	}

	type testcase struct {
		Class           string
		SampleRate      int
		ChannelConfig   string
		PerformanceMode string
	}

	testcaseToTestParams := func(tc testcase) []valMember {
		return []valMember{
			{
				Key:   "Class",
				Value: tc.Class,
			},
			{
				Key:   "SampleRate",
				Value: strconv.Itoa(tc.SampleRate),
			},
			{
				Key:   "ChannelConfig",
				Value: tc.ChannelConfig,
			},
			{
				Key:   "PerformanceMode",
				Value: tc.PerformanceMode,
			},
		}
	}

	var testcases []testcase

	// Different sample rate for stereo
	for _, sampleRate := range []int{8000, 11025, 16000, 22050, 32000, 44100, 48000} {
		testcases = append(testcases, testcase{
			Class:           classTestOutputSine,
			SampleRate:      sampleRate,
			ChannelConfig:   channelConfigOutStereo,
			PerformanceMode: performanceModeNone,
		})
	}

	// Different performance mode
	testcases = append(testcases, testcase{
		Class:           classTestOutputSine,
		SampleRate:      48000,
		ChannelConfig:   channelConfigOutStereo,
		PerformanceMode: performanceModePowerSaving,
	})
	testcases = append(testcases, testcase{
		Class:           classTestOutputSine,
		SampleRate:      48000,
		ChannelConfig:   channelConfigOutStereo,
		PerformanceMode: performanceModeLowLatency,
	})

	// Different channels configuration

	// 5.1 channels with different sample rates
	for _, sampleRate := range []int{8000, 11025, 16000, 22050, 32000, 44100, 48000} {
		testcases = append(testcases, testcase{
			Class:           classTestOutputSine,
			SampleRate:      sampleRate,
			ChannelConfig:   channelConfigOut5Point1,
			PerformanceMode: performanceModeNone,
		})
	}
	// Quad channels
	testcases = append(testcases, testcase{
		Class:           classTestOutputSine,
		SampleRate:      48000,
		ChannelConfig:   channelConfigOutQuad,
		PerformanceMode: performanceModeNone,
	})

	generateName := func(tierName string, tc testcase) string {
		var name []string
		if tierName != "" {
			name = append(name, tierName)
		}

		switch tc.ChannelConfig {
		case channelConfigOutStereo:
			name = append(name, "stereo")
		case channelConfigOutQuad:
			name = append(name, "quad")
		case channelConfigOut5Point1:
			name = append(name, "5point1")
		}

		name = append(name, strconv.Itoa(tc.SampleRate))

		switch tc.PerformanceMode {
		case performanceModeLowLatency:
			name = append(name, "lowlatency")
		case performanceModePowerSaving:
			name = append(name, "powersaving")
		}
		return strings.Join(name, "_")
	}

	var params []paramData

	for _, tier := range []struct {
		name                 string
		hwdep                string
		attr                 string
		incorrectSlicesLimit int
	}{{ // Stable device tier
		hwdep:                `hwdep.D(hwdep.Model(stableModel...))`,
		incorrectSlicesLimit: 50,
	}, { // Unstable device tier (Default for new devices)
		name:                 "unstable",
		hwdep:                `hwdep.D(hwdep.SkipOnModel(stableModel...))`,
		attr:                 `[]string{"informational"}`,
		incorrectSlicesLimit: 50,
	}} {
		for _, tc := range testcases {
			params = append(params, paramData{
				Name: generateName(tier.name, tc),
				Val: val{
					ArcaudioTestParams:   testcaseToTestParams(tc),
					IncorrectSlicesLimit: tier.incorrectSlicesLimit,
				},
				ExtraHardwareDeps: tier.hwdep,
				ExtraAttr:         tier.attr,
				Fixture:           "arcBooted",
			})
		}
	}

	// Generate Field-trial on and off variant for these testcases (only on stable tier)
	fieldTrialTestcases := []testcase{
		{
			Class:           classTestOutputSine,
			SampleRate:      48000,
			ChannelConfig:   channelConfigOutStereo,
			PerformanceMode: performanceModeNone,
		},
		{
			Class:           classTestOutputSine,
			SampleRate:      48000,
			ChannelConfig:   channelConfigOutQuad,
			PerformanceMode: performanceModeNone,
		},
		{
			Class:           classTestOutputSine,
			SampleRate:      48000,
			ChannelConfig:   channelConfigOut5Point1,
			PerformanceMode: performanceModeNone,
		},
		{
			Class:           classTestOutputSine,
			SampleRate:      48000,
			ChannelConfig:   channelConfigOutStereo,
			PerformanceMode: performanceModeLowLatency,
		},
		{
			Class:           classTestOutputSine,
			SampleRate:      48000,
			ChannelConfig:   channelConfigOutStereo,
			PerformanceMode: performanceModePowerSaving,
		},
	}
	for _, tc := range fieldTrialTestcases {
		// Field-trial config off
		paramOff := paramData{
			Name: generateName("", tc) + "_fieldtrial_testing_config_off",
			Val: val{
				ArcaudioTestParams:   testcaseToTestParams(tc),
				IncorrectSlicesLimit: 50,
			},
			ExtraHardwareDeps: `hwdep.D(hwdep.Model(stableModel...))`,
			Fixture:           "arcBootedWithFieldTrialConfigOff",
		}
		params = append(params, paramOff)

		// Field-trial config on
		paramOn := paramData{
			Name: generateName("", tc) + "_fieldtrial_testing_config_on",
			Val: val{
				ArcaudioTestParams:   testcaseToTestParams(tc),
				IncorrectSlicesLimit: 50,
			},
			ExtraHardwareDeps: `hwdep.D(hwdep.Model(stableModel...))`,
			Fixture:           "arcBootedWithFieldTrialConfigOn",
		}
		params = append(params, paramOn)
	}

	code := genparams.Template(t, `{{ range . }}{
		Name: {{ .Name | fmt }},
		ExtraHardwareDeps: {{ .ExtraHardwareDeps }},
		{{if .ExtraAttr}} ExtraAttr: {{ .ExtraAttr }}, {{end}}
		Val: audioLoopbackCorrectnessVal{
			arcaudioTestParams: arcaudio.TestParameters{
				{{ range .Val.ArcaudioTestParams }}{{ .Key }}: {{ .Value }},
				{{ end }}
			},
			incorrectSlicesLimit: {{ .Val.IncorrectSlicesLimit }},
		},
		{{if .Fixture}} Fixture: "{{ .Fixture }}", {{end}}
	},
	{{ end }}`, params)
	genparams.Ensure(t, "audio_loopback_correctness.go", code)
}

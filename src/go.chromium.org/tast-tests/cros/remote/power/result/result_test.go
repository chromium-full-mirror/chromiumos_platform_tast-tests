// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package result

import (
	"encoding/json"

	gotesting "testing"
)

func TestMarshalResult(t *gotesting.T) {
	fullResult := Result{
		FormatVersion: FormatVersion,
		Name:          "LowEndChromebook",
		Version:       "1.0",
		Personas: []Persona{
			{
				Name:    "MKT",
				Power:   Power{Average{MinutesBatteryLife: 320.5, MinutesBatteryLifeTested: 120, DischargeRate: 5.5}},
				Skipped: []string{"power.VideoPlayback.1080p_vp9"},
				Tests: []Test{
					{"power.Browsing", 0.6, Power{Average{MinutesBatteryLife: 320.5, MinutesBatteryLifeTested: 60, DischargeRate: 5.5}}},
					{"power.VideoPlayback.1080p_h264", 0.2, Power{Average{MinutesBatteryLife: 320.5, MinutesBatteryLifeTested: 60, DischargeRate: 5.5}}},
				},
			},
			{
				Name:    "EDU",
				Power:   Power{Average{MinutesBatteryLife: 320.5, MinutesBatteryLifeTested: 180, DischargeRate: 5.5}},
				Skipped: []string{"power.VideoPlayback.1080p_vp9"},
				Tests: []Test{
					{"power.Browsing", 0.4, Power{Average{MinutesBatteryLife: 320.5, MinutesBatteryLifeTested: 60, DischargeRate: 5.5}}},
					{"power.VideoPlayback.1080p_h264", 0.2, Power{Average{MinutesBatteryLife: 320.5, MinutesBatteryLifeTested: 60, DischargeRate: 5.5}}},
					{"power.YoutubeArc.1080p", 0.2, Power{Average{MinutesBatteryLife: 320.5, MinutesBatteryLifeTested: 60, DischargeRate: 5.5}}},
				},
			},
			{
				Name:    "VideoPlayback",
				Power:   Power{Average{MinutesBatteryLife: 320.5, MinutesBatteryLifeTested: 60, DischargeRate: 5.5}},
				Skipped: []string{"power.VideoPlayback.1080p_vp9"},
				Tests: []Test{
					{"power.VideoPlayback.1080p_h264", 0.5, Power{Average{MinutesBatteryLife: 320.5, MinutesBatteryLifeTested: 60, DischargeRate: 5.5}}},
				},
			},
		},
	}
	expected := exampleResult
	if json, err := json.MarshalIndent(fullResult, "", "\t"); err != nil {
		t.Fatal("failed to marshal fullResult: ", err)
	} else if string(json) != expected {
		t.Errorf("wrong result of fullConfig marshalling: expect %v, got %v", expected, string(json))
	}
}

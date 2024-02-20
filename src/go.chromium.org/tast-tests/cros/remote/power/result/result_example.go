// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package result

// exampleResult demonstrates a full result example in JSON.
const exampleResult = `{
	"format_version": 1,
	"config_name": "LowEndChromebook",
	"config_version": "1.0",
	"persona": [
		{
			"name": "MKT",
			"power": {
				"average": {
					"minutes_battery_life": 320.5,
					"minutes_battery_life_tested": 120,
					"watt_discharge_rate": 5.5
				}
			},
			"skipped_tests": [
				"power.VideoPlayback.1080p_vp9"
			],
			"tests": [
				{
					"name": "power.Browsing",
					"weight": 0.6,
					"power": {
						"average": {
							"minutes_battery_life": 320.5,
							"minutes_battery_life_tested": 60,
							"watt_discharge_rate": 5.5
						}
					}
				},
				{
					"name": "power.VideoPlayback.1080p_h264",
					"weight": 0.2,
					"power": {
						"average": {
							"minutes_battery_life": 320.5,
							"minutes_battery_life_tested": 60,
							"watt_discharge_rate": 5.5
						}
					}
				}
			]
		},
		{
			"name": "EDU",
			"power": {
				"average": {
					"minutes_battery_life": 320.5,
					"minutes_battery_life_tested": 180,
					"watt_discharge_rate": 5.5
				}
			},
			"skipped_tests": [
				"power.VideoPlayback.1080p_vp9"
			],
			"tests": [
				{
					"name": "power.Browsing",
					"weight": 0.4,
					"power": {
						"average": {
							"minutes_battery_life": 320.5,
							"minutes_battery_life_tested": 60,
							"watt_discharge_rate": 5.5
						}
					}
				},
				{
					"name": "power.VideoPlayback.1080p_h264",
					"weight": 0.2,
					"power": {
						"average": {
							"minutes_battery_life": 320.5,
							"minutes_battery_life_tested": 60,
							"watt_discharge_rate": 5.5
						}
					}
				},
				{
					"name": "power.YoutubeArc.1080p",
					"weight": 0.2,
					"power": {
						"average": {
							"minutes_battery_life": 320.5,
							"minutes_battery_life_tested": 60,
							"watt_discharge_rate": 5.5
						}
					}
				}
			]
		},
		{
			"name": "VideoPlayback",
			"power": {
				"average": {
					"minutes_battery_life": 320.5,
					"minutes_battery_life_tested": 60,
					"watt_discharge_rate": 5.5
				}
			},
			"skipped_tests": [
				"power.VideoPlayback.1080p_vp9"
			],
			"tests": [
				{
					"name": "power.VideoPlayback.1080p_h264",
					"weight": 0.5,
					"power": {
						"average": {
							"minutes_battery_life": 320.5,
							"minutes_battery_life_tested": 60,
							"watt_discharge_rate": 5.5
						}
					}
				}
			]
		}
	]
}`

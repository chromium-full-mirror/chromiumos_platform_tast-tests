// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

// exampleConfig demonstrates a full config example in JSON.
const exampleConfig = `
{
	"format_version": 1,
	"config_name": "LowEndChromebook",
	"config_version": "1.0",
	"test_control": {
		"max_duration": 60,
		"retry": 1,
		"fail_on_skipped_test": false
	},
	"persona": [
		{
			"name": "MKT",
			"tests": [
				{
					"name": "power.Browsing",
					"weight": 0.6
				},
				{
					"name": "power.VideoPlayback.1080p_vp9",
					"weight": 0.2
				},
				{
					"name": "power.VideoPlayback.1080p_h264",
					"weight": 0.2
				}
			]
		},
		{
			"name": "EDU",
			"tests": [
				{
					"name": "power.Browsing",
					"weight": 0.4
				},
				{
					"name": "power.VideoPlayback.1080p_vp9",
					"weight": 0.2
				},
				{
					"name": "power.VideoPlayback.1080p_h264",
					"weight": 0.2
				},
				{
					"name": "power.YoutubeArc.1080p",
					"weight": 0.2
				}
			]
		},
		{
			"name": "VideoPlayback",
			"tests": [
				{
					"name": "power.VideoPlayback.1080p_vp9",
					"weight": 0.5
				},
				{
					"name": "power.VideoPlayback.1080p_h264",
					"weight": 0.5
				}
			]
		}
	]
}
`

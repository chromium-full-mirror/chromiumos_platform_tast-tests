// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package syslog

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestExtractFileName(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    Entry
		want string
	}{
		{
			name: "NoFilename",
			e: Entry{
				Content: "dhcpcd exited",
			},
			want: "",
		},
		{
			name: "OneFilename",
			e: Entry{
				Content: "shill: [device.cc(1446)] Device wlan0",
			},
			want: "device.cc",
		},
		{
			name: "TwoFilenames",
			e: Entry{
				Content: "shill: [error.cc(126)] [wifi_service.cc(690)]: WiFi",
			},
			want: "wifi_service.cc",
		},
		{
			name: "NonFilenameBrackets",
			e: Entry{
				Content: "dnsproxyd: [client.cc(557)] Device [/device/wlan0] has",
			},
			want: "client.cc",
		},
		{
			name: "NewStyleFilename",
			e: Entry{
				Content: "dnsproxyd: [../libchrome-0.0.1/platform2/libchrome/dbus/object_proxy.cc:590] Failed to",
			},
			want: "object_proxy.cc",
		},
		{
			name: "NewStyleTwoFilenames",
			e: Entry{
				Content: "shill: [../path/to/error.cc(126)] [path/to/wifi_service.cc(690)]: WiFi",
			},
			want: "wifi_service.cc",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := ExtractFileName(tc.e)
			if diff := cmp.Diff(f, tc.want); diff != "" {
				t.Errorf("Unexpected filename extracted (-got +want): %s", diff)
			}
		})
	}
}

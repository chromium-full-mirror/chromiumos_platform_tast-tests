// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseStringList(t *testing.T) {
	type testCase struct {
		pslParam  string
		expectErr bool
		expected  interface{}
	}
	for _, tc := range []testCase{
		{"[]", false, []interface{}(nil)},
		{"", true, nil},
		{`['foo', 'bar\'', 'ba\\z']`, false, []interface{}{"foo", "bar'", `ba\z`}},
		{`[['one', 'two'], ['three']]`, false, []interface{}{[]interface{}{"one", "two"}, []interface{}{"three"}}},
		{`["\x00\u0001\U00000002"]`, false, []interface{}{"\000\001\002"}},
		{`["\x4a\u1E0A\U0001F6B4"]`, false, []interface{}{"JḊ🚴"}},
		{`[('chg.*:\r\n\tvoltage = 8200mV', '8200')]`, false, []interface{}{[]interface{}{"chg.*:\r\n\tvoltage = 8200mV", "8200"}}},
		{`[19, 28, 37]`, false, []interface{}{19, 28, 37}},
		{`[
			{
			    "prefix": [
				"main",
				"",
				"ccd_ti50",
				"ccd_gsc"
			    ],
			    "type": "ccd_ti50",
			    "vendor_id": 6353,
			    "product_id": 20554,
			    "serial": "01818054-54283135",
			    "sysfs_path": "/sys/bus/usb/devices/1-1.3",
			    "root_hub_device": "servo_v4p1 (18d1:520d) SERVOV4P1-S-2107250640",
			    "child_devices": []
			},
			{
			    "prefix": [
				"root",
				"servo_v4p1"
			    ],
			    "type": "servo_v4p1",
			    "vendor_id": 6353,
			    "product_id": 21005,
			    "serial": "SERVOV4P1-S-2107250640",
			    "sysfs_path": "/sys/bus/usb/devices/1-1.5",
			    "root_hub_device": "servo_v4p1 (18d1:520d) SERVOV4P1-S-2107250640",
			    "child_devices": [
				"ccd_ti50 (18d1:504a) 01818054-54283135"
			    ]
			}
		    ]`, false, []interface{}{
			map[string]interface{}{
				"prefix":          []interface{}{string("main"), string(""), string("ccd_ti50"), string("ccd_gsc")},
				"type":            string("ccd_ti50"),
				"vendor_id":       int(6353),
				"product_id":      int(20554),
				"serial":          string("01818054-54283135"),
				"sysfs_path":      string("/sys/bus/usb/devices/1-1.3"),
				"root_hub_device": string("servo_v4p1 (18d1:520d) SERVOV4P1-S-2107250640"),
				"child_devices":   []interface{}(nil),
			},
			map[string]interface{}{
				"prefix":          []interface{}{string("root"), string("servo_v4p1")},
				"type":            string("servo_v4p1"),
				"vendor_id":       int(6353),
				"product_id":      int(21005),
				"serial":          string("SERVOV4P1-S-2107250640"),
				"root_hub_device": string("servo_v4p1 (18d1:520d) SERVOV4P1-S-2107250640"),
				"sysfs_path":      string("/sys/bus/usb/devices/1-1.5"),
				"child_devices":   []interface{}{string("ccd_ti50 (18d1:504a) 01818054-54283135")},
			},
		}},
		{`[("pd 0 hard\r\nec:~> '\\x1b'[6D'\\x1b'[J[1078.462100 C0: PE_SNK_Hard_Reset", None, 'PE_SNK_Hard_Reset')]`, false, []interface{}{
			[]interface{}{
				string("pd 0 hard\r\nec:~> '\\x1b'[6D'\\x1b'[J[1078.462100 C0: PE_SNK_Hard_Reset"),
				nil,
				string("PE_SNK_Hard_Reset"),
			}},
		},
	} {
		res, err := ParseStringList(tc.pslParam)
		if tc.expectErr {
			if err == nil {
				t.Errorf("ParseStringList(%q) unexpectedly succeeded", tc.pslParam)
			}
		} else if err != nil {
			t.Errorf("ParseStringList(%q) failed %s", tc.pslParam, err)
		} else if !cmp.Equal(tc.expected, res) {
			t.Errorf("ParseStringList(%q) %s", tc.pslParam, cmp.Diff(tc.expected, res))
		}
	}
}

func TestPropertyToString(t *testing.T) {
	type testCase struct {
		propertyParam interface{}
		nameParam     string
		expectErr     bool
		expected      string
	}
	for _, tc := range []testCase{
		{[]interface{}(nil), "", true, "[]"},
		{map[string]interface{}{
			"type": string("servo_v4p1"),
		}, "serial", true, "servo_v4p1"},
		{map[string]interface{}{
			"product_id": int(21005),
		}, "product_id", true, "servo_v4p1"},
		{map[string]interface{}{
			"type": string("servo_v4p1"),
		}, "type", false, "servo_v4p1"},
	} {
		res, err := PropertyToString(tc.propertyParam, tc.nameParam)
		if tc.expectErr {
			if err == nil {
				t.Errorf("PropertyToString(%q, %q) unexpectedly succeeded", tc.propertyParam, tc.nameParam)
			}
		} else if err != nil {
			t.Errorf("PropertyToString(%q, %q) failed %s", tc.propertyParam, tc.nameParam, err)
		} else if !cmp.Equal(tc.expected, res) {
			t.Errorf("PropertyToString(%q, %q) %s", tc.propertyParam, tc.nameParam, cmp.Diff(tc.expected, res))
		}
	}
}

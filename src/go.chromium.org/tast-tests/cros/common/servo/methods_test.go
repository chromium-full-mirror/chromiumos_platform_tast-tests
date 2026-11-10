// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

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

func TestToggleOffOnRestoresOnContextTimeout_b584002432(t *testing.T) {
	type xmlCall struct {
		MethodName string   `xml:"methodName"`
		Params     []string `xml:"params>param>value>string"`
	}
	var mu sync.Mutex
	controls := map[string]string{string(DutEthPwrEn): string(On)}
	var setLog []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var call xmlCall
		_ = xml.Unmarshal(body, &call)
		w.Header().Set("Content-Type", "text/xml")
		mu.Lock()
		defer mu.Unlock()
		switch call.MethodName {
		case "echo":
			fmt.Fprintf(w, "<methodResponse><params><param><value><string>ECH0ING: %s</string></value></param></params></methodResponse>", call.Params[0])
		case "get":
			fmt.Fprintf(w, "<methodResponse><params><param><value><string>%s</string></value></param></params></methodResponse>", controls[call.Params[0]])
		case "set":
			controls[call.Params[0]] = call.Params[1]
			setLog = append(setLog, call.Params[0]+":"+call.Params[1])
			fmt.Fprint(w, "<methodResponse><params><param><value><boolean>1</boolean></value></param></params></methodResponse>")
		}
	}))
	defer srv.Close()

	host, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	svo, err := New(t.Context(), host, port)
	if err != nil {
		t.Fatalf("servo.New failed: %v", err)
	}
	defer svo.Close(context.Background())

	// Expire mid-sleep (30ms < ServoKeypressDelay=100ms) to reproduce b/584002432.
	shortCtx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if err := svo.ToggleOffOn(shortCtx, DutEthPwrEn); err == nil {
		t.Fatal("ToggleOffOn unexpectedly succeeded on expired context")
	}

	mu.Lock()
	gotState := controls[string(DutEthPwrEn)]
	gotLog := append([]string(nil), setLog...)
	mu.Unlock()

	if gotState != string(On) {
		t.Fatalf("dut_eth_pwr_en stranded %q after context timeout (sets: %v); want %q", gotState, gotLog, On)
	}
	wantLog := []string{"dut_eth_pwr_en:off", "dut_eth_pwr_en:on"}
	if !cmp.Equal(wantLog, gotLog) {
		t.Fatalf("unexpected set sequence (-want +got):\n%s", cmp.Diff(wantLog, gotLog))
	}
}

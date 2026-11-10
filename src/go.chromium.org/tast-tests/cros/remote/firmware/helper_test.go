// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/servo"
)

type fakeServod struct {
	srv      *httptest.Server
	mu       sync.Mutex
	controls map[string]string
	setLog   []string
}

func newFakeServod(initial map[string]string) *fakeServod {
	fs := &fakeServod{
		controls: make(map[string]string),
	}
	for k, v := range initial {
		fs.controls[k] = v
	}
	type xmlCall struct {
		MethodName string   `xml:"methodName"`
		Params     []string `xml:"params>param>value>string"`
	}
	writeFault := func(w http.ResponseWriter, msg string) {
		fmt.Fprintf(w, "<methodResponse><fault><value><struct><member><name>faultCode</name><value><int>1</int></value></member><member><name>faultString</name><value><string>%s</string></value></member></struct></value></fault></methodResponse>", msg)
	}
	fs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var call xmlCall
		_ = xml.Unmarshal(body, &call)
		w.Header().Set("Content-Type", "text/xml")

		fs.mu.Lock()
		defer fs.mu.Unlock()
		switch call.MethodName {
		case "echo":
			fmt.Fprintf(w, "<methodResponse><params><param><value><string>ECH0ING: %s</string></value></param></params></methodResponse>", call.Params[0])
		case "get_version":
			v := fs.controls["version"]
			if v == "" {
				v = "servo_v4p1_with_ccd_cr50"
			}
			fmt.Fprintf(w, "<methodResponse><params><param><value><string>%s</string></value></param></params></methodResponse>", v)
		case "doc":
			ctrl := call.Params[0]
			if _, ok := fs.controls[ctrl]; !ok {
				writeFault(w, "unknown control "+ctrl)
				return
			}
			fmt.Fprint(w, "<methodResponse><params><param><value><string>doc</string></value></param></params></methodResponse>")
		case "get":
			ctrl := call.Params[0]
			val, ok := fs.controls[ctrl]
			if !ok {
				writeFault(w, "unknown control "+ctrl)
				return
			}
			fmt.Fprintf(w, "<methodResponse><params><param><value><string>%s</string></value></param></params></methodResponse>", val)
		case "set":
			ctrl, val := call.Params[0], call.Params[1]
			fs.setLog = append(fs.setLog, ctrl+":"+val)
			fs.controls[ctrl] = val
			fmt.Fprint(w, "<methodResponse><params><param><value><boolean>1</boolean></value></param></params></methodResponse>")
		}
	}))
	return fs
}

func (fs *fakeServod) spec() string {
	_, port, _ := net.SplitHostPort(fs.srv.Listener.Addr().String())
	return ":" + port
}

func (fs *fakeServod) close() {
	fs.srv.Close()
}

func TestResetServoEthernetDongleExpiredContext_b584002432(t *gotesting.T) {
	fs := newFakeServod(map[string]string{
		string(servo.DutEthPwrEn): string(servo.On),
	})
	defer fs.close()

	h := NewHelperWithoutDUT("", fs.spec(), "", "")
	if err := h.RequireServo(t.Context()); err != nil {
		t.Fatalf("RequireServo failed: %v", err)
	}
	defer h.CloseServo(context.Background())

	// Reproduce RunBypasserUntilDUTConnected's 3s connectTimeout expiring right as
	// ResetServoEthernetDongle enters ToggleOffOn.
	shortCtx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if err := h.ResetServoEthernetDongle(shortCtx); err != nil {
		t.Fatalf("ResetServoEthernetDongle failed under short caller context: %v", err)
	}

	fs.mu.Lock()
	gotState := fs.controls[string(servo.DutEthPwrEn)]
	gotLog := append([]string(nil), fs.setLog...)
	fs.mu.Unlock()

	if gotState != string(servo.On) {
		t.Fatalf("dut_eth_pwr_en = %q; want %q", gotState, servo.On)
	}
	wantLog := []string{"dut_eth_pwr_en:off", "dut_eth_pwr_en:on"}
	if !cmp.Equal(wantLog, gotLog) {
		t.Fatalf("unexpected set sequence (-want +got):\n%s", cmp.Diff(wantLog, gotLog))
	}
}

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
	faultOn  map[string]bool
	setLog   []string
	onSet    func(ctrl, val string, controls map[string]string) error
}

func newFakeServod(initial map[string]string) *fakeServod {
	fs := &fakeServod{
		controls: make(map[string]string),
		faultOn:  make(map[string]bool),
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
			if fs.faultOn[ctrl+":"+val] {
				writeFault(w, "fault on "+ctrl+":"+val)
				return
			}
			if fs.onSet != nil {
				if err := fs.onSet(ctrl, val, fs.controls); err != nil {
					writeFault(w, err.Error())
					return
				}
			} else {
				fs.controls[ctrl] = val
			}
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

func TestRestoreServoPower(t *gotesting.T) {
	fs := newFakeServod(map[string]string{
		string(servo.DUTConnectionType): string(servo.DUTConnTypeC),
		string(servo.DTSMode):           string(servo.On),
		string(servo.PDRole):            string(servo.PDRoleSrc),
		string(servo.UARTRegexp):        "None",
		string(servo.UARTCmd):           "[['cc: off\\r\\n', 'off']]",
	})
	fs.onSet = func(ctrl, val string, controls map[string]string) error {
		if ctrl == string(servo.UARTCmd) && val == "cc off" {
			controls[ctrl] = "[['cc: off\\r\\n', 'off']]"
			return nil
		}
		controls[ctrl] = val
		return nil
	}
	defer fs.close()

	h := NewHelperWithoutDUT("", fs.spec(), "", "")
	defer h.CloseServo(context.Background())

	// Fast path: already src + dts:on must not bounce CC or issue set commands.
	if err := h.RestoreServoPower(t.Context()); err != nil {
		t.Fatalf("RestoreServoPower failed on healthy servo: %v", err)
	}
	fs.mu.Lock()
	if len(fs.setLog) != 0 {
		t.Fatalf("expected zero set calls when already src+dts:on, got %v", fs.setLog)
	}
	fs.controls[string(servo.PDRole)] = string(servo.PDRoleSnk)
	fs.controls[string(servo.DTSMode)] = string(servo.Off)
	fs.mu.Unlock()

	// Stranded snk/dts:off path: must restore dts:on and pd_role:src.
	if err := h.RestoreServoPower(t.Context()); err != nil {
		t.Fatalf("RestoreServoPower failed from snk: %v", err)
	}
	fs.mu.Lock()
	gotDTS := fs.controls[string(servo.DTSMode)]
	gotRole := fs.controls[string(servo.PDRole)]
	fs.mu.Unlock()
	if gotDTS != string(servo.On) || gotRole != string(servo.PDRoleSrc) {
		t.Fatalf("after RestoreServoPower: dts=%q role=%q; want on, src", gotDTS, gotRole)
	}
}

func TestEnsureDUTBootedWakesFromResetErrorViaPowerKey(t *gotesting.T) {
	fs := newFakeServod(map[string]string{
		string(servo.DUTConnectionType):  string(servo.DUTConnTypeC),
		string(servo.DTSMode):            string(servo.Off),
		string(servo.PDRole):             string(servo.PDRoleSnk),
		string(servo.UARTRegexp):         "None",
		string(servo.UARTCmd):            "[['cc: off\\r\\n', 'off']]",
		string(servo.ECSystemPowerState): "G3",
		string(servo.GSCCCDLevel):        servo.Open,
	})
	fs.faultOn["power_state:reset"] = true
	fs.faultOn["power_state:on"] = true
	fs.onSet = func(ctrl, val string, controls map[string]string) error {
		if ctrl == string(servo.UARTCmd) && val == "cc off" {
			controls[ctrl] = "[['cc: off\\r\\n', 'off']]"
			return nil
		}
		controls[ctrl] = val
		if ctrl == string(servo.PowerKey) {
			controls[string(servo.ECSystemPowerState)] = "S0"
		}
		return nil
	}
	defer fs.close()

	h := NewHelperWithoutDUT("", fs.spec(), "", "")
	defer h.CloseServo(context.Background())
	// Short context cancels waitDutS0's 20s post-S0 sleep once S0 is reached.
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	_ = h.EnsureDUTBooted(ctx)

	fs.mu.Lock()
	gotDTS := fs.controls[string(servo.DTSMode)]
	gotRole := fs.controls[string(servo.PDRole)]
	gotPwr := fs.controls[string(servo.ECSystemPowerState)]
	fs.mu.Unlock()

	if gotDTS != string(servo.On) || gotRole != string(servo.PDRoleSrc) {
		t.Fatalf("servo power not restored: dts=%q role=%q", gotDTS, gotRole)
	}
	if gotPwr != "S0" {
		t.Fatalf("DUT remained in %q; expected EnsureDUTBooted to fall back to PowerStateOn/PowerKey", gotPwr)
	}
}

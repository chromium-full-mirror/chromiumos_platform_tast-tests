// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"testing"

	"github.com/godbus/dbus/v5"
	"google.golang.org/protobuf/proto"

	tpmmanagerpb "go.chromium.org/chromiumos/system_api/tpm_manager_proto"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast/core/errors"
)

type fakeBusObject struct {
	dbus.BusObject
	call func(ctx context.Context, method string, flags dbus.Flags, args ...any) *dbus.Call
}

func (f *fakeBusObject) CallWithContext(ctx context.Context, method string, flags dbus.Flags, args ...any) *dbus.Call {
	return f.call(ctx, method, flags, args...)
}

func mustMarshalProto(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("proto.Marshal(%v) failed: %v", msg, err)
	}
	return b
}

func TestNonsensitiveStatusFromReply(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reply   *tpmmanagerpb.GetTpmNonsensitiveStatusReply
		want    hwsec.NonsensitiveStatusInfo
		wantErr bool
	}{
		{
			name: "is_enabled",
			reply: &tpmmanagerpb.GetTpmNonsensitiveStatusReply{
				Status:    tpmmanagerpb.TpmManagerStatus_STATUS_SUCCESS.Enum(),
				IsEnabled: proto.Bool(true),
			},
			want: hwsec.NonsensitiveStatusInfo{IsEnabled: true},
		},
		{
			name: "is_owned",
			reply: &tpmmanagerpb.GetTpmNonsensitiveStatusReply{
				Status:  tpmmanagerpb.TpmManagerStatus_STATUS_SUCCESS.Enum(),
				IsOwned: proto.Bool(true),
			},
			want: hwsec.NonsensitiveStatusInfo{IsOwned: true},
		},
		{
			name: "is_owner_password_present",
			reply: &tpmmanagerpb.GetTpmNonsensitiveStatusReply{
				Status:                 tpmmanagerpb.TpmManagerStatus_STATUS_SUCCESS.Enum(),
				IsOwnerPasswordPresent: proto.Bool(true),
			},
			want: hwsec.NonsensitiveStatusInfo{IsOwnerPasswordPresent: true},
		},
		{
			name: "has_reset_lock_permissions",
			reply: &tpmmanagerpb.GetTpmNonsensitiveStatusReply{
				Status:                  tpmmanagerpb.TpmManagerStatus_STATUS_SUCCESS.Enum(),
				HasResetLockPermissions: proto.Bool(true),
			},
			want: hwsec.NonsensitiveStatusInfo{HasResetLockPermissions: true},
		},
		{
			name: "error_status",
			reply: &tpmmanagerpb.GetTpmNonsensitiveStatusReply{
				Status: tpmmanagerpb.TpmManagerStatus_STATUS_DEVICE_ERROR.Enum(),
			},
			wantErr: true,
		},
		{
			name:    "missing_status",
			reply:   &tpmmanagerpb.GetTpmNonsensitiveStatusReply{},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := nonsensitiveStatusFromReply(tc.reply)
			if tc.wantErr {
				if err == nil {
					t.Errorf("nonsensitiveStatusFromReply(%v) = (%+v, nil), want non-nil error", tc.reply, got)
				}
				return
			}
			if err != nil {
				t.Errorf("nonsensitiveStatusFromReply(%v) returned unexpected error: %v", tc.reply, err)
				return
			}
			if *got != tc.want {
				t.Errorf("nonsensitiveStatusFromReply(%v) = %+v, want %+v", tc.reply, *got, tc.want)
			}
		})
	}
}

func TestDAInfoFromReply(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reply   *tpmmanagerpb.GetDictionaryAttackInfoReply
		want    hwsec.DAInfo
		wantErr bool
	}{
		{
			name: "success",
			reply: &tpmmanagerpb.GetDictionaryAttackInfoReply{
				Status:                                  tpmmanagerpb.TpmManagerStatus_STATUS_SUCCESS.Enum(),
				DictionaryAttackCounter:                 proto.Uint32(1),
				DictionaryAttackThreshold:               proto.Uint32(200),
				DictionaryAttackLockoutInEffect:         proto.Bool(true),
				DictionaryAttackLockoutSecondsRemaining: proto.Uint32(30),
			},
			want: hwsec.DAInfo{Counter: 1, Threshold: 200, InEffect: true, Remaining: 30},
		},
		{
			name: "error_status",
			reply: &tpmmanagerpb.GetDictionaryAttackInfoReply{
				Status: tpmmanagerpb.TpmManagerStatus_STATUS_DEVICE_ERROR.Enum(),
			},
			wantErr: true,
		},
		{
			name:    "missing_status",
			reply:   &tpmmanagerpb.GetDictionaryAttackInfoReply{},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := daInfoFromReply(tc.reply)
			if tc.wantErr {
				if err == nil {
					t.Errorf("daInfoFromReply(%v) = (%+v, nil), want non-nil error", tc.reply, got)
				}
				return
			}
			if err != nil {
				t.Errorf("daInfoFromReply(%v) returned unexpected error: %v", tc.reply, err)
				return
			}
			if *got != tc.want {
				t.Errorf("daInfoFromReply(%v) = %+v, want %+v", tc.reply, *got, tc.want)
			}
		})
	}
}

func TestTPMManagerDBusGetNonsensitiveStatusIgnoreCache(t *testing.T) {
	const wantMethod = "org.chromium.TpmManager.GetTpmNonsensitiveStatus"

	t.Run("success", func(t *testing.T) {
		replyBytes := mustMarshalProto(t, &tpmmanagerpb.GetTpmNonsensitiveStatusReply{
			Status:                  tpmmanagerpb.TpmManagerStatus_STATUS_SUCCESS.Enum(),
			IsEnabled:               proto.Bool(true),
			IsOwned:                 proto.Bool(true),
			IsOwnerPasswordPresent:  proto.Bool(false),
			HasResetLockPermissions: proto.Bool(true),
		})
		client := &TPMManagerDBus{obj: &fakeBusObject{
			call: func(ctx context.Context, method string, flags dbus.Flags, args ...any) *dbus.Call {
				if method != wantMethod {
					t.Errorf("D-Bus method = %q, want %q", method, wantMethod)
				}
				if len(args) != 1 {
					t.Fatalf("D-Bus args len = %d, want 1", len(args))
				}
				reqBytes, ok := args[0].([]byte)
				if !ok {
					t.Fatalf("D-Bus arg type = %T, want []byte", args[0])
				}
				var req tpmmanagerpb.GetTpmNonsensitiveStatusRequest
				if err := proto.Unmarshal(reqBytes, &req); err != nil {
					t.Fatalf("proto.Unmarshal(request) failed: %v", err)
				}
				if !req.GetIgnoreCache() {
					t.Errorf("request IgnoreCache = false, want true")
				}
				return &dbus.Call{Body: []any{replyBytes}}
			},
		}}

		got, err := client.GetNonsensitiveStatusIgnoreCache(t.Context())
		if err != nil {
			t.Fatalf("GetNonsensitiveStatusIgnoreCache() failed: %v", err)
		}
		want := hwsec.NonsensitiveStatusInfo{
			IsEnabled:               true,
			IsOwned:                 true,
			IsOwnerPasswordPresent:  false,
			HasResetLockPermissions: true,
		}
		if *got != want {
			t.Errorf("GetNonsensitiveStatusIgnoreCache() = %+v, want %+v", *got, want)
		}
	})

	for _, tc := range []struct {
		name string
		call *dbus.Call
	}{
		{name: "dbus_error", call: &dbus.Call{Err: errors.New("service unavailable")}},
		{name: "malformed_response", call: &dbus.Call{Body: []any{[]byte{0xff}}}},
		{name: "empty_response", call: &dbus.Call{Body: []any{[]byte{}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &TPMManagerDBus{obj: &fakeBusObject{
				call: func(ctx context.Context, method string, flags dbus.Flags, args ...any) *dbus.Call {
					return tc.call
				},
			}}
			if got, err := client.GetNonsensitiveStatusIgnoreCache(t.Context()); err == nil {
				t.Errorf("GetNonsensitiveStatusIgnoreCache() = (%+v, nil), want non-nil error", got)
			}
		})
	}
}

func TestTPMManagerDBusGetDAInfo(t *testing.T) {
	const wantMethod = "org.chromium.TpmManager.GetDictionaryAttackInfo"

	t.Run("success", func(t *testing.T) {
		replyBytes := mustMarshalProto(t, &tpmmanagerpb.GetDictionaryAttackInfoReply{
			Status:                                  tpmmanagerpb.TpmManagerStatus_STATUS_SUCCESS.Enum(),
			DictionaryAttackCounter:                 proto.Uint32(2),
			DictionaryAttackThreshold:               proto.Uint32(200),
			DictionaryAttackLockoutInEffect:         proto.Bool(false),
			DictionaryAttackLockoutSecondsRemaining: proto.Uint32(0),
		})
		client := &TPMManagerDBus{obj: &fakeBusObject{
			call: func(ctx context.Context, method string, flags dbus.Flags, args ...any) *dbus.Call {
				if method != wantMethod {
					t.Errorf("D-Bus method = %q, want %q", method, wantMethod)
				}
				if len(args) != 1 {
					t.Fatalf("D-Bus args len = %d, want 1", len(args))
				}
				reqBytes, ok := args[0].([]byte)
				if !ok {
					t.Fatalf("D-Bus arg type = %T, want []byte", args[0])
				}
				var req tpmmanagerpb.GetDictionaryAttackInfoRequest
				if err := proto.Unmarshal(reqBytes, &req); err != nil {
					t.Fatalf("proto.Unmarshal(request) failed: %v", err)
				}
				return &dbus.Call{Body: []any{replyBytes}}
			},
		}}

		got, err := client.GetDAInfo(t.Context())
		if err != nil {
			t.Fatalf("GetDAInfo() failed: %v", err)
		}
		want := hwsec.DAInfo{Counter: 2, Threshold: 200, InEffect: false, Remaining: 0}
		if *got != want {
			t.Errorf("GetDAInfo() = %+v, want %+v", *got, want)
		}
	})

	for _, tc := range []struct {
		name string
		call *dbus.Call
	}{
		{name: "dbus_error", call: &dbus.Call{Err: errors.New("service unavailable")}},
		{name: "malformed_response", call: &dbus.Call{Body: []any{[]byte{0xff}}}},
		{name: "empty_response", call: &dbus.Call{Body: []any{[]byte{}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &TPMManagerDBus{obj: &fakeBusObject{
				call: func(ctx context.Context, method string, flags dbus.Flags, args ...any) *dbus.Call {
					return tc.call
				},
			}}
			if got, err := client.GetDAInfo(t.Context()); err == nil {
				t.Errorf("GetDAInfo() = (%+v, nil), want non-nil error", got)
			}
		})
	}
}

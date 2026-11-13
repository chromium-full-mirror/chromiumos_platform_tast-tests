// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"

	"github.com/godbus/dbus/v5"
	"google.golang.org/protobuf/proto"

	tpmmanagerpb "go.chromium.org/chromiumos/system_api/tpm_manager_proto"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
)

const (
	tpmManagerName      = "org.chromium.TpmManager"
	tpmManagerPath      = dbus.ObjectPath("/org/chromium/TpmManager")
	tpmManagerInterface = "org.chromium.TpmManager"
)

// TPMManagerDBus talks to tpm_managerd via D-Bus APIs. It is faster than
// hwsec.TPMManagerClient, which runs tpm_manager_client for every call.
type TPMManagerDBus struct {
	obj dbus.BusObject
}

// NewTPMManagerDBus creates a TPMManagerDBus on the shared system bus
// connection. Unlike NewAttestationDBus, it does not wait for the service, so
// that calls fail immediately instead of blocking if tpm_managerd is not
// running.
func NewTPMManagerDBus() (*TPMManagerDBus, error) {
	conn, err := dbusutil.SystemBus()
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to system bus")
	}
	return &TPMManagerDBus{conn.Object(tpmManagerName, tpmManagerPath)}, nil
}

// GetNonsensitiveStatusIgnoreCache retrieves the NonsensitiveStatusInfo and
// ignores the cache, like "tpm_manager_client status --nonsensitive
// --ignore_cache".
func (t *TPMManagerDBus) GetNonsensitiveStatusIgnoreCache(ctx context.Context) (*hwsec.NonsensitiveStatusInfo, error) {
	req := &tpmmanagerpb.GetTpmNonsensitiveStatusRequest{IgnoreCache: proto.Bool(true)}
	var reply tpmmanagerpb.GetTpmNonsensitiveStatusReply
	if err := dbusutil.CallProtoMethod(ctx, t.obj, tpmManagerInterface+".GetTpmNonsensitiveStatus", req, &reply); err != nil {
		return nil, err
	}
	return nonsensitiveStatusFromReply(&reply)
}

// GetDAInfo retrieves the dictionary attack info, like
// "tpm_manager_client get_da_info".
func (t *TPMManagerDBus) GetDAInfo(ctx context.Context) (*hwsec.DAInfo, error) {
	req := &tpmmanagerpb.GetDictionaryAttackInfoRequest{}
	var reply tpmmanagerpb.GetDictionaryAttackInfoReply
	if err := dbusutil.CallProtoMethod(ctx, t.obj, tpmManagerInterface+".GetDictionaryAttackInfo", req, &reply); err != nil {
		return nil, err
	}
	return daInfoFromReply(&reply)
}

func nonsensitiveStatusFromReply(reply *tpmmanagerpb.GetTpmNonsensitiveStatusReply) (*hwsec.NonsensitiveStatusInfo, error) {
	// Status is a proto2 optional enum whose zero value is STATUS_SUCCESS,
	// so check presence explicitly to reject empty replies.
	if reply.Status == nil {
		return nil, errors.New("GetTpmNonsensitiveStatus reply missing status")
	}
	if s := reply.GetStatus(); s != tpmmanagerpb.TpmManagerStatus_STATUS_SUCCESS {
		return nil, errors.Errorf("GetTpmNonsensitiveStatus returned status %v", s)
	}
	return &hwsec.NonsensitiveStatusInfo{
		IsEnabled:               reply.GetIsEnabled(),
		IsOwned:                 reply.GetIsOwned(),
		IsOwnerPasswordPresent:  reply.GetIsOwnerPasswordPresent(),
		HasResetLockPermissions: reply.GetHasResetLockPermissions(),
	}, nil
}

func daInfoFromReply(reply *tpmmanagerpb.GetDictionaryAttackInfoReply) (*hwsec.DAInfo, error) {
	// Status is a proto2 optional enum whose zero value is STATUS_SUCCESS,
	// so check presence explicitly to reject empty replies.
	if reply.Status == nil {
		return nil, errors.New("GetDictionaryAttackInfo reply missing status")
	}
	if s := reply.GetStatus(); s != tpmmanagerpb.TpmManagerStatus_STATUS_SUCCESS {
		return nil, errors.Errorf("GetDictionaryAttackInfo returned status %v", s)
	}
	return &hwsec.DAInfo{
		Counter:   uint(reply.GetDictionaryAttackCounter()),
		Threshold: uint(reply.GetDictionaryAttackThreshold()),
		InEffect:  reply.GetDictionaryAttackLockoutInEffect(),
		Remaining: uint(reply.GetDictionaryAttackLockoutSecondsRemaining()),
	}, nil
}

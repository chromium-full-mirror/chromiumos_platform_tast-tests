// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vpn

import (
	"context"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"chromiumos/tast/local/shill"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const charonExitTimeout = 5 * time.Second

// waitForCharonExitOrKill waits until the charon process stopped after the
// disconnection of an strongswan-based connection. If the connection is not
// strongswan-based, returns nil immediately. If the charon process is still
// running after charonExitTimeout, this function will kill it directly and
// return an error. Reasons that this function is needed: 1) a leftover charon
// process may affect the following tests; 2) shill is supposed to stop the
// charon process properly after VPN is disconnected, if the charon process is
// still running after the test, it probably indicates an issue in shill or
// shill has crashed.
func waitForCharonExitOrKill(ctx context.Context) error {
	const pidFile = "/run/ipsec/charon.pid"

	// Assume charon is not running and return nil directly if either 1) pid file
	// does not exist, or 2) pid file does not contain a valid pid number.
	pidStr, err := ioutil.ReadFile(pidFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	pid, err := strconv.Atoi(strings.TrimRight(string(pidStr), "\n"))
	if err != nil {
		testing.ContextLogf(ctx, "Charon pid file has content `%s`, assume it is not running", string(pidStr))
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return errors.Wrapf(err, "failed to find process: %d", pid)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := process.Signal(unix.Signal(0)); err == nil {
			return errors.New("charon is still running")
		}
		return nil
	}, &testing.PollOptions{Timeout: charonExitTimeout}); err == nil {
		return nil
	}

	process.Kill()
	process.Wait()
	return errors.New("charon is still running")
}

// RemoveVPNProfile removes the VPN service with |name| if it exists in a
// best-effort way.
func RemoveVPNProfile(ctx context.Context, name string) error {
	findServiceProps := make(map[string]interface{})
	findServiceProps[shillconst.ServicePropertyName] = name
	findServiceProps[shillconst.ServicePropertyType] = shillconst.TypeVPN

	manager, err := shill.NewManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create shill manager proxy")
	}

	svc, err := manager.FindMatchingService(ctx, findServiceProps)
	if err != nil {
		if err.Error() == shillconst.ErrorMatchingServiceNotFound {
			return nil
		}
		return errors.Wrap(err, "failed to call FindMatchingService")
	}

	testing.ContextLog(ctx, "Removing VPN service ", svc)
	return svc.Remove(ctx)
}

// FindVPNService returns a VPN service matching the given |serviceGUID| in
// shill.
func FindVPNService(ctx context.Context, m *shill.Manager, serviceGUID string) (*shill.Service, error) {
	testing.ContextLog(ctx, "Trying to find service with guid ", serviceGUID)

	findServiceProps := make(map[string]interface{})
	findServiceProps["GUID"] = serviceGUID
	findServiceProps["Type"] = "vpn"
	service, err := m.WaitForServiceProperties(ctx, findServiceProps, 5*time.Second)
	if err == nil {
		testing.ContextLogf(ctx, "Found service %v matching guid %s", service, serviceGUID)
	}
	return service, err
}

// VerifyVPNServiceConnect verifies if |service| is connectable.
func VerifyVPNServiceConnect(ctx context.Context, m *shill.Manager, service *shill.Service) error {
	pw, err := service.CreateWatcher(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create watcher")
	}
	defer pw.Close(ctx)
	if err := service.Connect(ctx); err != nil {
		return errors.Wrapf(err, "failed to connect the service %v", service)
	}
	defer func() {
		if err = service.Disconnect(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to disconnect service ", service)
		}
	}()

	timeoutCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	state, err := pw.ExpectIn(timeoutCtx, shillconst.ServicePropertyState, append(shillconst.ServiceConnectedStates, shillconst.ServiceStateFailure))
	if err != nil {
		return err
	}

	if state == shillconst.ServiceStateFailure {
		return errors.Errorf("service %v became failure state", service)
	}
	return nil
}

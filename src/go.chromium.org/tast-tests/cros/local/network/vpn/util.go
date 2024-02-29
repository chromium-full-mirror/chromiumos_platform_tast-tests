// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vpn

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const charonExitTimeout = 5 * time.Second
const charonPath = "/usr/libexec/ipsec/charon"

// checkPidIsRunningCharon returns if the given pid is pointing to a charon
// process by reading its process path. It's possible that the pid file is just
// a leftover and there is another process using this pid.
func checkPidIsRunningCharon(ctx context.Context, pid int) (bool, error) {
	procPath, err := testexec.CommandContext(ctx, "readlink", fmt.Sprintf("/proc/%d/exe", pid)).Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			// The path does not exist. Probably means the process has stopped.
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to get process path for pid %d", pid)
	}
	if strings.TrimSpace(string(procPath)) != charonPath {
		testing.ContextLogf(ctx, "Charon pid file (pid=%d) points to another executable %s", pid, procPath)
		return false, nil
	}

	return true, nil
}

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

	const errMsgCharonIsRunning = "charon is still running"

	testing.ContextLogf(ctx, "Waiting for charon (pid=%d) to exit", pid)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		stillRunning, err := checkPidIsRunningCharon(ctx, pid)
		if err != nil {
			return testing.PollBreak(err)
		}
		if stillRunning {
			return errors.New(errMsgCharonIsRunning)
		}
		return nil
	}, &testing.PollOptions{Timeout: charonExitTimeout}); err == nil {
		// No error means there is no running charon process now.
		return nil
	} else if err.Error() != errMsgCharonIsRunning {
		return err
	}

	testing.ContextLog(ctx, "Charon is still running. Killing it")
	process.Kill()
	process.Wait()
	return errors.New(errMsgCharonIsRunning)
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

// SetAlwaysOnVPN configures shill to use Always-on VPN mode with vpnSvc. On
// success, returns a function to disable Always-on VPN (instead of restoring
// the state prior to the call to this function).
func SetAlwaysOnVPN(ctx context.Context, mode shillconst.AlwaysOnVPNMode, vpnSvc *shill.Service) (func(context.Context), error) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to shill Manager")
	}

	profile, err := m.ActiveProfile(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get active profile")
	}

	if err := profile.SetAlwaysOnVPN(ctx, mode, vpnSvc); err != nil {
		return nil, errors.Wrap(err, "failed to configure Always-on VPN in shill")
	}

	return func(context.Context) {
		if err := profile.SetAlwaysOnVPN(ctx, shillconst.AlwaysOnVPNModeOff, nil); err != nil {
			testing.ContextLog(ctx, "Failed to reset Always-on VPN in shill: ", err)
		}
	}, nil
}

// GetAlwaysOnVPNMode returns the current Always-on VPN mode configured in
// shill.
func GetAlwaysOnVPNMode(ctx context.Context) (shillconst.AlwaysOnVPNMode, error) {
	const retModeUnknown = shillconst.AlwaysOnVPNModeOff

	m, err := shill.NewManager(ctx)
	if err != nil {
		return retModeUnknown, errors.Wrap(err, "failed to connect to shill Manager")
	}

	profile, err := m.ActiveProfile(ctx)
	if err != nil {
		return retModeUnknown, errors.Wrap(err, "failed to get active profile")
	}

	props, err := profile.GetProperties(ctx)
	if err != nil {
		return retModeUnknown, errors.Wrapf(err, "failed to get properties from profile %s", profile.ObjectPath())
	}

	mode, err := props.GetString(shillconst.ProfilePropertyAlwaysOnVPNMode)
	if err != nil {
		return retModeUnknown, errors.Wrap(err, "failed to get Always-on VPN mode")
	}

	return shillconst.AlwaysOnVPNMode(mode), nil
}

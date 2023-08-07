// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"io"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/internal/config"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/driver"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/media/vm"
	"go.chromium.org/tast-tests/cros/local/network/dumputil"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

// loginUser logs in to a freshly-restarted Chrome instance.
// It waits for the login process to complete before returning.
func loginUser(ctx context.Context, cfg *config.Config, sess *driver.Session) error {
	conn, err := WaitForOOBEConnection(ctx, sess)
	if err != nil {
		return err
	}
	defer conn.Close()

	creds := cfg.Creds()

	testing.ContextLogf(ctx, "Logging in as user %q", creds.User)
	ctx, st := timing.Start(ctx, "login")
	defer st.End()

	switch cfg.LoginMode() {
	case config.FakeLogin:
		if err := conn.Call(ctx, nil, "Oobe.loginForTesting", creds.User, creds.Pass, creds.GAIAID, false); err != nil {
			return err
		}
	case config.GAIALogin, config.SAMLLogin:
		// GAIA login requires Internet connectivity.
		if err := shill.WaitForOnline(ctx); err != nil {
			if !vm.IsRunningOnVM() {
				if pingErr := ping.VerifyInternetConnectivity(ctx, 5*time.Second); pingErr != nil {
					testing.ContextLog(ctx, "Failed to wait for shill online test: ", err)

					dumpfile := "network_dump_ping_" + time.Now().Format("030405000") + ".txt"
					if err := dumputil.DumpNetworkInfo(ctx, dumpfile); err != nil {
						testing.ContextLog(ctx, "Failed to dump network info after a ping expectation failure: ", err)
					}
					testing.ContextLog(ctx, "Ping expectation failed, current network info dumped into ", dumpfile)

					return errors.Wrap(pingErr, "pre login network connection tests failed")
				}
			}

			// Fail even if the ping test was successful.
			// TODO(b/268671917): Remove double check when the root cause is clear.
			testing.ContextLog(ctx, "Calling shill.WaitForOnline failed, but ping.VerifyInternetConnectivity passed")
			return err
		}
		if err := performGAIALogin(ctx, cfg, sess, conn); err != nil {
			return err
		}
	}

	return nil
}

// waitForCryptohome waits for user's encrypted home directory to be mounted and
// informs about occurred errors.
func waitForCryptohome(ctx context.Context, cfg *config.Config) error {
	mountType := cryptohome.Permanent
	if cfg.EphemeralUser() {
		mountType = cryptohome.Ephemeral
	}

	// Shorten deadline to reserve time for reading the log.
	shortenCtx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// TODO(b/257471572): remove the deadline log once the bug is resoloved.
	if deadline, ok := ctx.Deadline(); ok {
		testing.ContextLogf(ctx, "Deadline: %s", deadline)
	}

	if err := cryptohome.WaitForUserMountAndValidateType(shortenCtx, cfg.NormalizedUser(), mountType); err != nil {
		if cfg.LoginMode() == config.GAIALogin || cfg.LoginMode() == config.SAMLLogin {
			// Backup the original error.
			origErr := err

			// Check the error message from the server side.
			logReader, err := syslog.NewLineReader(ctx, syslog.ChromeLogFile, true, nil)
			if err != nil {
				return errors.Wrapf(origErr, "could not get Chrome log reader: %v", err)
			}
			defer logReader.Close()

			for {
				line, err := logReader.ReadLine()
				if err != nil {
					if err != io.EOF {
						return errors.Wrapf(origErr, "failed to read file %v: %v", syslog.ChromeLogFile, err)
					}

					// Could not find server side authentication error.
					// Return the error directly.
					return origErr
				}
				if strings.HasSuffix(line, "Got authentication error\n") {
					break
				}
			}

			// Skip two lines after authentication error to identify the exact authentication error:
			//   Got authentication error
			//   net_error: net::OK (skip)
			//   response body: { (skip)
			//    "error": "rate_limit_exceeded"
			//   }
			for i := 0; i < 2; i++ {
				if _, err := logReader.ReadLine(); err != nil {
					return errors.Wrapf(origErr, "failed to skip the lines after authentication error: %v", err)
				}
			}

			// Read the third line after the authentication error.
			line, err := logReader.ReadLine()
			if err != nil {
				return errors.Wrapf(origErr, "failed to read the authentication error: %v", err)
			}
			authErr := strings.TrimSpace(line)
			return errors.Errorf("authentication error: %v", authErr)
		}
		return err
	}

	return nil
}

// removeNotifications clears all notifications after logging in so none will be
// shown at the beginning of tests.
func removeNotifications(ctx context.Context, sess *driver.Session) error {
	// TODO(crbug/1079235): move this outside of the switch once the test API is available in guest mode.
	tconn, err := sess.TestAPIConn(ctx, true)
	if err != nil {
		return err
	}
	if err := tconn.Eval(ctx, "tast.promisify(chrome.autotestPrivate.removeAllNotifications)()", nil); err != nil {
		return errors.Wrap(err, "failed to clear notifications")
	}

	return nil
}

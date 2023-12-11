// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package secagentd

import (
	rep "chromiumos/reporting"
	xdr "chromiumos/xdr/secagentd"
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdcommon"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentddbusmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdupstart"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/proto"
)

const (
	defaultUser = "testuser@gmail.com"
	defaultPass = "testpass"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AuthenticationEvents,
		Desc: "Checks that XDR authentication events are correctly being reported",
		Contacts: []string{
			"cros-enterprise-security@google.com",
			"rborzello@google.com",
			"aashay@google.com",
			"jasonling@google.com",
		},
		// ChromeOS > Security > ChromeOS Enterprise Security
		BugComponent: "b:1208373",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"bpf", "chrome"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// AuthenticationEvents triggers User events (logging in/out, lock/unlock)
// and verifies it against the events emitted by secagentd over dbus.
func AuthenticationEvents(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 15*time.Second)
	defer cancel()
	// Restart with default parameter.
	defer secagentdupstart.RestartSecagentd(cleanupCtx)

	// Restart chrome with the authenticate event feature.
	cr, err := chrome.New(ctx, chrome.EnableFeatures("CrOSLateBootSecagentdXDRAuthenticateEvents"), chrome.DeferLogin())
	if err != nil {
		s.Fatal("Failed to restart chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	const batchIntervalS = 5
	// Restart secagentd and have it ignore policy and not wait for the first
	// agent event to be enqueued successfully.
	agentPid, err := secagentdupstart.RestartSecagentd(ctx,
		upstart.WithArg("SECAGENTD_LOG_LEVEL", "-1"),
		upstart.WithArg("BYPASS_POLICY_FOR_TESTING", "true"),
		upstart.WithArg("BYPASS_ENQ_OK_WAIT_FOR_TESTING", "true"),
		upstart.WithArg("PLUGIN_BATCH_INTERVAL_S_FOR_TESTING", strconv.Itoa(batchIntervalS)))
	if err != nil {
		s.Fatal("Failed to restart secagentd: ", err)
	}

	ew, cancel, err := secagentddbusmonitor.SetupDbusWatcherWithTimeout(ctx, agentPid, 60*time.Second)
	if err != nil {
		s.Fatal("Failed to setup dbus monitoring: ", err)
	}
	defer cancel()

	// Open a keyboard device.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open keyboard device: ", err)
	}
	defer kb.Close(cleanupCtx)

	// Create expected login.
	expLogin := xdr.AuthenticateEvent{
		Authentication: &xdr.Authentication{
			AuthFactor: []xdr.Authentication_AuthenticationType{xdr.Authentication_AUTH_NEW_USER},
		},
	}

	// Create expected unlock.
	expUnlock := xdr.AuthenticateEvent{
		Authentication: &xdr.Authentication{
			AuthFactor: []xdr.Authentication_AuthenticationType{xdr.Authentication_AUTH_PASSWORD},
		},
	}

	// 1: Login.
	if err = cr.ContinueLogin(ctx); err != nil {
		s.Fatal("Failed to log in: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// 2: Lock.
	if err := lockscreen.Lock(ctx, tconn); err != nil {
		s.Fatal("Failed to lock the screen: ", err)
	}

	// 3: Auth Failures.
	expFailures := 3
	for i := 0; i < expFailures; i++ {
		if err = lockscreen.EnterPassword(ctx, tconn, defaultUser, "Wrong Password", kb); err != nil {
			s.Error("Failed to enter password credentials: ", err)
		}
	}

	// 4: Unlock.
	if err := lockscreen.UnlockWithPassword(ctx, tconn, defaultUser, defaultPass, kb, 10*time.Second, 30*time.Second); err != nil {
		s.Fatal("Failed to unlock the screen: ", err)
	}

	// 5: Logout
	if err := quicksettings.SignOut(ctx, tconn); err != nil {
		s.Fatal("Failed to logout: ", err)
	}

	login, lock, unlock, logout := false, false, false, false
	actFailures := 0
	deviceUser := ""

	for {
		events, err := checkAuthenticationEventWatcher(s, ew)
		if err != nil {
			s.Error("Error checking event watcher: ", err)
			break
		}

		for _, event := range events.GetBatchedEvents() {
			if err := secagentdcommon.CheckCommon(event.GetCommon()); err != nil {
				s.Error("Invalid common field: ", err)
			}
			// Verify that all device users UUID are the same because it is same account.
			if deviceUser == "" {
				deviceUser = *event.Common.DeviceUser
			} else if *event.Common.DeviceUser != "" && *event.Common.DeviceUser != deviceUser {
				s.Errorf("Device user does not match. Expected: %s, Actual: %s", deviceUser, *event.Common.DeviceUser)
			}

			if event.GetLogon() != nil {
				login = true

				// The auth factor will sometimes report password and sometimes nothing.
				// Just check that it is filled because the most important part is verifying that the login event was sent.
				if len(event.GetLogon().Authentication.AuthFactor) == 0 ||
					(event.GetLogon().Authentication.AuthFactor[0] != xdr.Authentication_AUTH_NEW_USER) {
					s.Errorf("Logon event failed to match. Expected: %s, Actual: %s", expLogin.String(), event.String())
				}
			} else if event.GetLock() != nil {
				lock = true
			} else if event.GetUnlock() != nil {
				unlock = true
				if !proto.Equal(event.GetUnlock(), &expUnlock) {
					s.Errorf("Unlock event failed to match. Expected: %s, Actual: %s", expUnlock.String(), event.String())
				}
			} else if event.GetLogoff() != nil {
				logout = true
			} else if event.GetFailure() != nil {
				// Because of the short batch interval the auth failures might be split.
				actFailures += int(*event.GetFailure().Authentication.NumFailedAttempts)
			}
		}

		if logout && unlock && lock && login && actFailures == expFailures {
			break
		}
	}

	if deviceUser == "" {
		s.Error("Device user never filled")
	}

	if actFailures != expFailures {
		s.Errorf("Incorrect number of failure events. Expected: %d, Actual: %d", expFailures, actFailures)
	}

	if !logout || !unlock || !lock || !login {
		s.Errorf("Not all events found. Logout: %t Unlock: %t Lock: %t Login: %t", logout, unlock, lock, login)
	}
}

func checkAuthenticationEventWatcher(s *testing.State, ew *dbusutil.EventWatcher) (*xdr.XdrUserEvent, error) {
	event, ok := <-ew.Events()
	if !ok {
		return nil, errors.New("Timed out waiting for expected events")
	}
	if len(event.Arguments) == 0 {
		return nil, nil
	}
	arg, ok := event.Arguments[0].([]byte)
	if !ok {
		return nil, nil
	}
	enq := &rep.EnqueueRecordRequest{}
	if err := proto.Unmarshal(arg, enq); err != nil {
		s.Fatal("Failed to unmarshal an EnqueueRecordRequest: ", err)
	}

	if enq.GetRecord().GetDestination() != rep.Destination_CROS_SECURITY_USER {
		return nil, nil
	}
	ae := &xdr.XdrUserEvent{}
	if err := proto.Unmarshal(enq.GetRecord().GetData(), ae); err != nil {
		s.Fatal("Failed to unmarshal data for a CROS_SECURITY_USER record: ", err)
	}

	s.Log("Snooped XdrUserEvent: ", ae.String())

	return ae, nil
}

// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package secagentd

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	rep "go.chromium.org/chromiumos/reporting"
	xdr "go.chromium.org/chromiumos/xdr/secagentd"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdaffiliation"
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
		Attr:         []string{"group:mainline", "group:enterprise-reporting"},
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
	defer secagentdupstart.RestartSecagentd(cleanupCtx, false)

	cr, err := chrome.New(ctx, chrome.DeferLogin())
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cr.Close(cleanupCtx)
	const batchIntervalS = 5

	currTime := time.Now()
	// Restart secagentd and have it ignore policy and not wait for the first
	// agent event to be enqueued successfully.
	// Have it wait for the AddMatch signal.
	agentPid, err := secagentdupstart.RestartSecagentd(ctx, true,
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

	// Create expected unlock.
	expUnlock := xdr.AuthenticateEvent{
		Authentication: &xdr.Authentication{
			AuthFactor: []xdr.Authentication_AuthenticationType{xdr.Authentication_AUTH_PASSWORD},
		},
	}

	// Poll until secagentd is ready to listen to session manager.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := secagentdaffiliation.GetSessionManagerReady(ctx, currTime)
		return err
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		s.Error("Failed to listen to session manager: ", err)
	}

	// 1: Login.
	if err = cr.ContinueLogin(ctx); err != nil {
		s.Fatal("Failed to log in: ", err)
	}

	// Poll for signedInUser affiliation directory being created.
	signedInUser, hash, err := secagentdaffiliation.GetSignedInUser(ctx)
	if err != nil {
		s.Error("Failed to get signed in user: ", err)
	}
	deviceUser := ""
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, deviceUser, err = secagentdaffiliation.GetAffiliationStatus(signedInUser, hash)
		return err
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Error("Failed to get affiliation status: ", err)
	}

	// 2: Lock.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

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

	for {
		events, err := checkAuthenticationEventWatcher(s, ew)
		if err != nil {
			s.Error("Error checking event watcher: ", err)
			break
		}

		for _, event := range events.GetBatchedEvents() {
			checkAuthCommon(s, event, deviceUser)

			if event.GetLogon() != nil {
				if login {
					s.Errorf("Extra Login event found: %s", event)
				}
				login = true
				// The auth factor will sometimes report password and sometimes new user depending on the existing state of the device.
				if len(event.GetLogon().Authentication.AuthFactor) == 0 ||
					(event.GetLogon().Authentication.AuthFactor[0] != xdr.Authentication_AUTH_NEW_USER && event.GetLogon().Authentication.AuthFactor[0] != xdr.Authentication_AUTH_PASSWORD) {
					s.Errorf("Logon event failed to match. Got: %s, Want: AUTH_NEW_USER or AUTH_PASSWORD", event.String())
				}
			} else if event.GetLock() != nil {
				if lock {
					s.Errorf("Extra Lock event found: %s", event)
				}
				lock = true
			} else if event.GetUnlock() != nil {
				if unlock {
					s.Errorf("Extra Unlock event found: %s", event)
				}
				unlock = true
				if !proto.Equal(event.GetUnlock(), &expUnlock) {
					s.Errorf("Unlock event failed to match. Got: %s, Want: %s", event.String(), expUnlock.String())
				}
			} else if event.GetLogoff() != nil {
				if logout {
					s.Errorf("Extra Logout event found: %s", event)
				}
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
		s.Errorf("Incorrect number of failure events. Got: %d, Want: %d", actFailures, expFailures)
	}

	if !logout || !unlock || !lock || !login {
		s.Errorf("Not all events found. Logout: %t Unlock: %t Lock: %t Login: %t", logout, unlock, lock, login)
	}
}

func checkAuthCommon(s *testing.State, event *xdr.UserEventAtomicVariant, deviceUser string) {
	if err := secagentdcommon.CheckCommon(event.GetCommon()); err != nil {
		s.Errorf("Invalid common field: %s\t Event: %s", err, event.String())
	} else if *event.Common.DeviceUser == "" {
		s.Error("Device user is empty")
	} else if !strings.HasPrefix(*event.Common.DeviceUser, "UnaffiliatedUser-") {
		s.Errorf("Device user does not have unaffiliated prefix. Got: %s", *event.Common.DeviceUser)
	} else if _, err := uuid.Parse((*event.Common.DeviceUser)[len("UnaffiliatedUser-"):]); err != nil {
		s.Errorf("Device user does not contain valid UUID. Got: %s", *event.Common.DeviceUser)
	}

	// Verify that all device users UUID are the same because it is same account.
	if deviceUser != *event.Common.DeviceUser {
		s.Errorf("Device user does not match. Got: %s, Want: %s", *event.Common.DeviceUser, deviceUser)
	}

}

func checkAuthenticationEventWatcher(s *testing.State, ew *dbusutil.EventWatcher) (*xdr.XdrUserEvent, error) {
	event, ok := <-ew.Events()
	if !ok {
		return nil, errors.New("Timed out waiting for Want events")
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

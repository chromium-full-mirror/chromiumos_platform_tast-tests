// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package session

import (
	"bytes"
	"context"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	empb "chromiumos/policy/chromium/policy/enterprise_management_proto"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OwnershipRetaken,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensures that ownership is re-taken upon loss of owner's cryptohome and device policies are preserved",
		Contacts: []string{
			"hidehiko@chromium.org",
			"miersh@google.com",
		},
		// ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		BugComponent: "b:1207311",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational", "group:cq-medium", "group:hw_agnostic"},
	})
}

// prepareForSessionManagerNotifications returns an accessor for session manager
// methods, signal watchers for device policies, and owner key update events.
func prepareForSessionManagerNotifications(ctx context.Context) (
	*session.SessionManager, *dbusutil.SignalWatcher, *dbusutil.SignalWatcher, error) {
	sessionManager, err := session.NewSessionManager(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to create session_manager binding")
	}
	settingsWatcher, err := sessionManager.WatchPropertyChangeComplete(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to start watching PropertyChangeComplete signal")
	}
	keyWatcher, err := sessionManager.WatchSetOwnerKeyComplete(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to start watching SetOwnerKeyComplete signal")
	}
	return sessionManager, settingsWatcher, keyWatcher, nil
}

// readPolicyValues returns device policy values (i.e. the content of device
// policies).
func readPolicyValues(ctx context.Context, sessionManager *session.SessionManager) (
	*empb.ChromeDeviceSettingsProto, error) {
	policyValues, err := session.RetrieveSettings(ctx, sessionManager)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve settings")
	}
	return policyValues, nil
}

// readPolicyProto returns full device policy protobuf message (that includes
// a signature by the owner key).
func readPolicyProto(ctx context.Context, sessionManager *session.SessionManager) (
	*empb.PolicyFetchResponse, error) {
	policyProto, err := sessionManager.RetrievePolicyEx(ctx, session.DevicePolicyDescriptor())
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve policies")
	}
	return policyProto, nil
}

// readPublicKey returns the public half of the owner key.
func readPublicKey(ctx context.Context) ([]byte, error) {
	path := filepath.Join(session.PolicyPath, "owner.key")
	pubKey, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read owner key")
	}
	if len(pubKey) == 0 {
		return nil, errors.Wrap(err, "public key is empty")
	}

	return pubKey, nil
}

func OwnershipRetaken(ctx context.Context, s *testing.State) {
	const (
		testUser = "ownership_test@chromium.org"
		testPass = "testme"
	)

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	policyValues1, policyProto1, pubKey1 := func() (
		*empb.ChromeDeviceSettingsProto,
		*empb.PolicyFetchResponse, []byte) {
		sessionManager, settingsWatcher, keyWatcher, err := prepareForSessionManagerNotifications(ctx)
		if err != nil {
			s.Fatal("Failed to prepare for session manager notifications: ", err)
		}
		defer keyWatcher.Close(cleanupCtx)
		defer settingsWatcher.Close(cleanupCtx)

		// Create a new user that will take ownership.
		cr, err := chrome.New(ctx, chrome.FakeLogin(chrome.Creds{User: testUser, Pass: testPass}))
		if err != nil {
			s.Fatal("Failed to log in with Chrome: ", err)
		}
		defer cr.Close(cleanupCtx)

		// Wait until the creation of initial policies and the owner key.
		select {
		case <-keyWatcher.Signals:
		case <-ctx.Done():
			s.Fatal("Timed out waiting for initial key creation: ", ctx.Err())
		}
		select {
		case <-settingsWatcher.Signals:
		case <-ctx.Done():
			s.Fatal("Timed out waiting for initial policy creation: ", ctx.Err())
		}

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect to test API: ", err)
		}
		ui := uiauto.New(tconn)

		// Change system settings, so they are different from the initial ones.
		// This change is expected to be preserved when ChromeOS retakes ownership.
		toggle24Hours := nodewith.Name("Use 24-hour clock").Role(role.ToggleButton)

		settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "dateTime", ui.Exists(toggle24Hours))
		if err != nil {
			s.Fatal("Failed to open time and date OS settings: ", err)
		}
		defer settings.Close(cleanupCtx)

		if err := ui.DoDefault(toggle24Hours)(ctx); err != nil {
			s.Fatal("Failed to toggle 24-hour clock: ", err)
		}

		// Wait until the change is saved on disk by session manager.
		settingsWatcher2, err := sessionManager.WatchPropertyChangeComplete(ctx)
		if err != nil {
			s.Fatal("Failed to start watching PropertyChangeComplete signal: ", err)
		}
		select {
		case <-settingsWatcher2.Signals:
		case <-ctx.Done():
			s.Fatal("Timed out waiting for policy update: ", ctx.Err())
		}

		// Retrieve current policies and the key.
		policyValues, err := readPolicyValues(ctx, sessionManager)
		if err != nil {
			s.Fatal("Failed to read policy values: ", err)
		}
		policyProto, err := readPolicyProto(ctx, sessionManager)
		if err != nil {
			s.Fatal("Failed to read proto: ", err)
		}
		publicKey, err := readPublicKey(ctx)
		if err != nil {
			s.Fatal("Failed to read public key: ", err)
		}

		return policyValues, policyProto, publicKey
	}()

	// Emulate logout.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to log out: ", err)
	}

	// Delete user data for the owner user, including the private half of the
	// owner key. This will make ChromeOS retake ownership when the owner user
	// signs in next time.
	if err := cryptohome.RemoveVault(ctx, testUser); err != nil {
		s.Fatal("Failed to remove vault: ", err)
	}

	policyValues2, policyProto2, pubKey2 := func() (
		*empb.ChromeDeviceSettingsProto,
		*empb.PolicyFetchResponse, []byte) {
		sessionManager, settingsWatcher, keyWatcher, err := prepareForSessionManagerNotifications(ctx)
		if err != nil {
			s.Fatal("Failed to prepare for session manager notifications: ", err)
		}
		defer keyWatcher.Close(cleanupCtx)
		defer settingsWatcher.Close(cleanupCtx)

		// Sign in to retake ownership.
		cr, err := chrome.New(ctx, chrome.KeepState(), chrome.TryReuseSession(),
			chrome.FakeLogin(chrome.Creds{User: testUser, Pass: testPass}))
		if err != nil {
			s.Fatal("Failed to log in with Chrome: ", err)
		}
		defer cr.Close(cleanupCtx)

		// Wait until a new owner key is generated and policies and the key are
		// updated by session manager.
		select {
		case <-keyWatcher.Signals:
		case <-ctx.Done():
			s.Fatal("Timed out waiting for key update: ", ctx.Err())
		}
		select {
		case <-settingsWatcher.Signals:
		case <-ctx.Done():
			s.Fatal("Timed out waiting for policy update: ", ctx.Err())
		}

		// Retrieve updated policies and the key.
		policyValues, err := readPolicyValues(ctx, sessionManager)
		if err != nil {
			s.Fatal("Failed to read policy values: ", err)
		}
		policyProto, err := readPolicyProto(ctx, sessionManager)
		if err != nil {
			s.Fatal("Failed to read proto: ", err)
		}
		publicKey, err := readPublicKey(ctx)
		if err != nil {
			s.Fatal("Failed to read public key: ", err)
		}

		return policyValues, policyProto, publicKey
	}()

	if bytes.Equal(pubKey1, pubKey2) {
		s.Fatal("Owner key should have changed")
	}

	// The full protobuf message should have changed because it's now signed by
	// the new key.
	if diff := cmp.Diff(policyProto1, policyProto2, protocmp.Transform()); diff == "" {
		s.Fatal("The signature in the policy protobuf message should have changed")
	}

	// But policy values should stay the same.
	if diff := cmp.Diff(policyValues1, policyValues2, protocmp.Transform()); diff != "" {
		const diffName = "diff.txt"
		if err := ioutil.WriteFile(filepath.Join(s.OutDir(), diffName), []byte(diff), 0644); err != nil {
			s.Error("Failed to write diff: ", err)
		}
		s.Error("Policy values changed after the ownership was retaken, diff can be found in ", diffName)
	}
}

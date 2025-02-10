// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package smartcard provides utility functions for running smartcard tast tests.
package smartcard

import (
	"context"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/services/cros/peripherals"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

// Account holds the data of a smart card account that can be used in tests.
type Account struct {
	Username string
	PinCode  string
}

// SmartCard holds the resources required for running smart card tests.
type SmartCard struct {
	svc            peripherals.PeriphServiceClient
	currentAccount Account
	account1       Account
	account2       Account
	manualTest     bool
}

const (
	wrongPinCode                = "111111"
	removalNotificationDuration = 15 // The setting unit is seconds.
	manaulTestTimeout           = 10 * time.Second
)

// New creates a new SmartCard instance.
func New(ctx context.Context, cl *rpc.Client, manualTest bool, account1, account2 Account) (*SmartCard, error) {
	svc := peripherals.NewPeriphServiceClient(cl.Conn)
	if _, err := svc.NewSmartCard(ctx, &empty.Empty{}); err != nil {
		return nil, errors.Wrap(err, "failed to create smart card")
	}
	return &SmartCard{
		svc:        svc,
		manualTest: manualTest,
		account1:   account1,
		account2:   account2,
	}, nil
}

// CleanupSmartCard performs the cleanup of the smart card.
func (s *SmartCard) CleanupSmartCard(ctx context.Context) error {
	if _, err := s.svc.CleanupSmartCard(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to create smart card")
	}
	return nil
}

// FirstTimeLogin returns an action that inserts the smart card,
// sets a wrong PIN, then sets the correct PIN to login.
//
// Procedure:
// 1. Enroll device with "account@managedchrome.com" / <password>.
// 2. Insert the smart card into the reader if it already isn't inserted.
// 3. Click "Next", and verify that the ADFS page is opened.
// 4. Click "Sign in using a certificate" link on ADFS page.
// 5. Enter a wrong PIN and verify the error message is displayed.
// 6. Enter the correct PIN.
func (s *SmartCard) FirstTimeLogin() action.Action {
	return action.Combine("first time login",
		s.insertSmartCard(),
		s.setWrongPIN(),
		s.setCorrectPIN(),
	)
}

// AddNewUser returns an action that adds a new user by inserting the second
// smart card and setting PINs.
//
// Procedure:
// 1. Insert the 2nd smart card into the reader if it already isn't inserted.
// 2. User clicks on "Add Person" button.
// 3. Click "Sign in using a certificate" link on ADFS page.
// 4. Enter a wrong PIN and verify the error message is displayed.
// 5. Enter the correct PIN.
func (s *SmartCard) AddNewUser() action.Action {
	return action.Combine("add new user",
		s.signOut(),
		s.insertSecondSmartCard(),
		s.addPerson(),
		s.setWrongPIN(),
		s.setCorrectPIN(),
	)
}

// ReAuthenticationOnline returns an action for online re-authentication.
//
// Procedure:
//  1. User logs out of the session if already signed in
//     -> Click on Sign out button.
//  2. Click on the right arrow "->" button to sign in.
//  3. Enter correct PIN and verify that the sign-in completes and the user
//     session starts.
func (s *SmartCard) ReAuthenticationOnline() action.Action {
	return s.reAuthentication(false)
}

// ReAuthenticationOffline returns an action for offline re-authentication.
//
// Procedure:
//  1. User logs out of the session if already signed in
//     -> Click on Sign out button.
//  2. Disable Wifi or unplug the Ethernet cable.
//  3. Click on the right arrow "->" button to sign in.
//  4. Enter correct PIN and verify that the sign-in completes and the user
//     session starts.
func (s *SmartCard) ReAuthenticationOffline() action.Action {
	return s.reAuthentication(true)
}

// UnlockScreenWithSmartCardOnline returns an action that locks the screen
// and test whether removing/inserting a smart card has the expected
// behavior online.
//
// Procedure:
//  1. Lock the screen and verify that no password/PIN field is displayed on
//     the user’s pod.
//  2. Remove the card from the reader.
//  3. Start the unlock attempt by clicking the button on the user pod and
//     verify that login fails.
//  4. Insert the card back.
//  5. Repeat the sign-in attempt and verify that the PIN request appears.
//  6. Enter a wrong PIN and verify the error message is displayed.
//  7. Enter the correct PIN and verify that the user session gets unlocked.
func (s *SmartCard) UnlockScreenWithSmartCardOnline() action.Action {
	return s.unlockScreenTesting(false)
}

// UnlockScreenWithSmartCardOffline returns an action that locks the screen
// and test whether removing/inserting a smart card has the expected
// behavior offline.
//
// Procedure:
//  1. Lock the screen and verify that no password/PIN field is displayed on
//     the user’s pod.
//  2. Disable Ethernet and verify device is disconnected from internet.
//  3. Remove the card from the reader.
//  4. Start the unlock attempt by clicking the button on the user pod and
//     verify that login fails.
//  5. Insert the card back.
//  6. Repeat the sign-in attempt and verify that the PIN request appears.
//  7. Enter a wrong PIN and verify the error message is displayed.
//  8. Enter the correct PIN and verify that the user session gets unlocked.
func (s *SmartCard) UnlockScreenWithSmartCardOffline() action.Action {
	return s.unlockScreenTesting(true)
}

// AutomaticLockOnSmartCardRemoval returns an action that sets security
// policy to "Lock the current session" and removal notification duration
// to 15s. And verify that the settings are valid.
//
// Procedure:
//  1. Use incognito session to login to admin.google.com.
//  2. Set Chrome>Settings>User & Browser settings>
//     Security: Security token removal = "Lock the current session"
//     & Removal notification duration to 15
//     NOTE: managedchrome.com>croscommidentity-adfs> smartcard already is
//     pre-configured with these settings.
//  3. Click on the right arrow "->" button to sign in.
//  4. Insert the smart card into the reader if it already isn't inserted.
//  5. Enter the correct PIN and click on the Submit button.
//     Verify that the user session started.
//  6. Remove the smart card.
//     Verify that notification is triggered informing the user
//     "Your Chromebook will be locked automatically in 15 seconds".
//  7. Verify that the user’s screen automatically locked after 15 seconds.
func (s *SmartCard) AutomaticLockOnSmartCardRemoval() action.Action {
	return action.Named("automatic lock on smart card removal",
		s.automaticActionOnSmartCardRemoval(securityPolicyLock),
	)
}

// AutomaticLogoutOnSmartCardRemoval returns an action that sets security
// policy to "Log the user out" and removal notification duration
// to 15s. And verify that the settings are valid.
//
// Procedure:
//  1. Use incognito session to login to admin.google.com.
//  2. Set Chrome>Settings>User & Browser settings>
//     Security: Security token removal = ""Log the user out""
//     & Removal notification duration to 15
//     NOTE: managedchrome.com>croscommidentity-adfs> smartcard already is
//     pre-configured with these settings.
//  3. Click on the right arrow “->” button to sign in.
//  4. Insert the smart card into the reader if it already isn't inserted.
//  5. Enter the correct PIN and click on the Submit button.
//     Verify that the user session started.
//  6. Remove the smart card.
//     Verify that notification is triggered informing the user
//     "You will be automatically signed out in 15 seconds".
//  7. Verify that the user is automatically logged out after 15 seconds.
func (s *SmartCard) AutomaticLogoutOnSmartCardRemoval() action.Action {
	return action.Named("automatic log out on smart card removal",
		s.automaticActionOnSmartCardRemoval(securityPolicyLogOut),
	)
}

// WebApplicationAuthentication returns an action that performs web
// application authentication.
//
// Procedure:
// 1. Open an incognito window (via Shift+Ctrl+N) and navigate to:
// mail.google.com
// 2. Type the test smart card account’s email:
// scard-lon@managedchrome.com (No password required)
// 3. Click on "Sign in using a certificate".
// 4. Click "OK" to proceed with it.
// 5. If requested, enter the PIN "000000".
func (s *SmartCard) WebApplicationAuthentication() action.Action {
	return func(ctx context.Context) error {
		if _, err := s.svc.WebApplicationAuthentication(ctx,
			&peripherals.WebApplicationAuthenticationRequest{
				Email:   s.currentAccount.Username + "@managedchrome.com",
				PinCode: s.currentAccount.PinCode,
			}); err != nil {
			return errors.Wrap(err, "failed to perform web application authentication")
		}
		return nil
	}
}

// DriveLockCSSIAppTesting returns an action that performs Drivelock CSSI app
// testing.
//
// Procedure:
// 1. Launch the Drivelock CSSI app and verify that it displays the connected reader.
// 2. Press "Test now".
// 3. Enter the PIN when prompted and verify that the test succeeded with "Signing: OK".
func (s *SmartCard) DriveLockCSSIAppTesting() action.Action {
	return func(ctx context.Context) error {
		if _, err := s.svc.DriveLockCSSIAppTesting(ctx,
			&peripherals.SetPINRequest{PinCode: s.currentAccount.PinCode},
		); err != nil {
			return errors.Wrap(err, "failed to perform Drivelock CSSI app testing")
		}
		return nil
	}
}

// VerifyCertificate returns an action that verifies the certificate for the
// current account.
//
// Procedure:
// 1. Open chrome://settings/certificates in browser
// 2. Verify that the smart card certificate appears.
func (s *SmartCard) VerifyCertificate() action.Action {
	return func(ctx context.Context) error {
		if _, err := s.svc.VerifyCertificate(ctx,
			&peripherals.VerifyCertificateRequest{Username: s.currentAccount.Username},
		); err != nil {
			return errors.Wrap(err, "failed to verify certificate")
		}
		return nil
	}
}

// insertSmartCard returns an action that inserts the smart card.
func (s *SmartCard) insertSmartCard() action.Action {
	return func(ctx context.Context) error {
		if s.manualTest {
			testing.ContextLog(ctx, "Please insert smart card in ", manaulTestTimeout)
			// GoBigSleepLint: Wait for the smart card to be inserted manually
			// within the timeout period.
			if err := testing.Sleep(ctx, manaulTestTimeout); err != nil {
				return errors.Wrap(err, "failed to sleep")
			}
		} else {
			// TODO: Insert the first smart card by robotic arm .
		}

		s.currentAccount = s.account1
		return nil
	}
}

// insertSecondSmartCard returns an action that inserts the second smart card.
func (s *SmartCard) insertSecondSmartCard() action.Action {
	return func(ctx context.Context) error {
		if s.manualTest {
			testing.ContextLog(ctx, "Please insert the second smart card in ", manaulTestTimeout)
			// GoBigSleepLint: Wait for the second smart card to be inserted
			// manually within the timeout period.
			if err := testing.Sleep(ctx, manaulTestTimeout); err != nil {
				return errors.Wrap(err, "failed to sleep")
			}
		} else {
			// TODO: Insert the second smart card by robotic arm .
		}

		s.currentAccount = s.account2
		return nil
	}
}

// removeSmartCard returns an action that removes the smart card.
func (s *SmartCard) removeSmartCard() action.Action {
	return func(ctx context.Context) error {
		if s.manualTest {
			testing.ContextLog(ctx, "Please remove smart card in ", manaulTestTimeout)
			// GoBigSleepLint: Wait for the smart card to be removed manually
			// within the timeout period.
			if err := testing.Sleep(ctx, manaulTestTimeout); err != nil {
				return errors.Wrap(err, "failed to sleep")
			}
		} else {
			// TODO: Remove smart card by robotic arm .
		}
		return nil
	}
}

// setCorrectPIN returns an action that sets correct PIN code and verifies
// the login status is true.
func (s *SmartCard) setCorrectPIN() action.Action {
	return func(ctx context.Context) error {
		correctPinCode := s.currentAccount.PinCode
		res, err := s.svc.SetPIN(ctx, &peripherals.SetPINRequest{PinCode: correctPinCode})
		if err != nil {
			return errors.Wrapf(err, "failed to set PIN code %q", correctPinCode)
		}
		if res.Err != "" {
			return errors.Errorf("unexpected error message, got %q want %q", res.Err, "")
		}
		return nil
	}
}

// setWrongPIN returns an action that sets wrong PIN code and verifies the
// error message "Invalid PIN." is displayed.
func (s *SmartCard) setWrongPIN() action.Action {
	return func(ctx context.Context) error {
		const expectedErrorMessage = "Invalid PIN."
		res, err := s.svc.SetPIN(ctx, &peripherals.SetPINRequest{PinCode: wrongPinCode})
		if err != nil {
			return errors.Wrapf(err, "failed to set PIN code %q", wrongPinCode)
		}
		if res.Err != expectedErrorMessage {
			return errors.Errorf("unexpected error message, got %q want %q", res.Err, expectedErrorMessage)
		}
		return nil
	}
}

// reAuthentication returns an action that signs out and signs in, with an
// optional offline mode.
func (s *SmartCard) reAuthentication(offline bool) action.Action {
	return func(ctx context.Context) error {
		if _, err := s.svc.ReAuthentication(ctx,
			&peripherals.ReAuthenticationRequest{
				Offline: offline,
				PinCode: s.currentAccount.PinCode,
			}); err != nil {
			return errors.Wrap(err, "failed to re-authentication")
		}
		return nil
	}
}

// addPerson returns an action that adds a person.
func (s *SmartCard) addPerson() action.Action {
	return func(ctx context.Context) error {
		if _, err := s.svc.AddPerson(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to add person")
		}
		return nil
	}
}

// signOut returns an action that signs out smart card.
func (s *SmartCard) signOut() action.Action {
	return func(ctx context.Context) error {
		if _, err := s.svc.SmartCardSignOut(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to sign out smart card")
		}
		return nil
	}
}

// signIn returns an action that signs in smart card.
func (s *SmartCard) signIn() action.Action {
	return func(ctx context.Context) error {
		if _, err := s.svc.SmartCardSignIn(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to sign in smart card")
		}
		return nil
	}
}

// unlockScreenTesting locks screen, then concurrently tests smart card removal and insertion,
// as well as smart card functionality, with optional offline mode.
func (s *SmartCard) unlockScreenTesting(offline bool) action.Action {
	return func(ctx context.Context) error {
		if _, err := s.svc.LockScreen(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to lock screen")
		}

		var wg sync.WaitGroup
		var removeAndInsertSmartCardErr, smartCardRemovalAndInsertionTestingErr error

		wg.Add(2)

		go func() {
			defer wg.Done()

			waitForSignIn := func(ctx context.Context) error {
				const signInTimeout = 10 * time.Second
				testing.ContextLogf(ctx, "wait %v for the smart card to sign in", signInTimeout)
				// GoBigSleepLint: Wait for the smart card to sign in.
				if err := testing.Sleep(ctx, signInTimeout); err != nil {
					return errors.Wrap(err, "failed to sleep")
				}
				return nil
			}

			removeAndInsertSmartCardErr = action.Combine("remove and insert smart card",
				s.removeSmartCard(),
				waitForSignIn,
				s.insertSecondSmartCard(),
			)(ctx)
		}()

		go func() {
			defer wg.Done()
			_, smartCardRemovalAndInsertionTestingErr = s.svc.SmartCardRemovalAndInsertionTesting(ctx,
				&peripherals.SmartCardRemovalAndInsertionTestingRequest{
					Offline:        offline,
					WrongPinCode:   wrongPinCode,
					CorrectPinCode: s.currentAccount.PinCode,
				})
		}()

		wg.Wait()

		if removeAndInsertSmartCardErr != nil {
			return errors.Wrap(removeAndInsertSmartCardErr, "failed to remove and insert smart card")
		}

		if smartCardRemovalAndInsertionTestingErr != nil {
			return errors.Wrap(smartCardRemovalAndInsertionTestingErr, "failed to perform smart card removal and insertion testing")
		}

		return nil
	}
}

// securityPolicyOption represents the different security options that can be applied.
type securityPolicyOption string

const (
	// securityPolicyLogOut indicates that the user should be logged out.
	securityPolicyLogOut securityPolicyOption = "Log the user out"
	// securityPolicyLock indicates that the current user session should be locked.
	securityPolicyLock securityPolicyOption = "Lock the current session"
)

// setSecurityTokenRemoval returns an action that sets the security policy
// for token removal (log out or lock session). It sends a request to
// configure the policy with the specified option and duration.
func (s *SmartCard) setSecurityTokenRemoval(option securityPolicyOption) action.Action {
	return func(ctx context.Context) error {
		if _, err := s.svc.SetSecurityTokenRemoval(ctx, &peripherals.SetSecurityTokenRemovalRequest{
			SecurityPolicyOption: string(option),
			Duration:             removalNotificationDuration,
		}); err != nil {
			return errors.Wrap(err, "failed to set security token removal")
		}
		return nil
	}
}

// waitForAutoLockOrLogOut returns an action that removes the smart card
// and waits for either auto-lock or log out.
func (s *SmartCard) waitForAutoLockOrLogOut(option securityPolicyOption) action.Action {
	return func(ctx context.Context) error {
		var wg sync.WaitGroup
		var removeSmartCardErr, waitForAutoLockOrLogOutErr error

		wg.Add(2)

		go func() {
			defer wg.Done()
			removeSmartCardErr = s.removeSmartCard()(ctx)
		}()

		go func() {
			defer wg.Done()
			_, waitForAutoLockOrLogOutErr = s.svc.WaitForAutoLockOrLogOut(ctx,
				&peripherals.SetSecurityTokenRemovalRequest{
					SecurityPolicyOption: string(option),
					Duration:             removalNotificationDuration,
				})
		}()

		wg.Wait()

		if removeSmartCardErr != nil {
			return errors.Wrap(removeSmartCardErr, "failed to remove smart card")
		}

		if waitForAutoLockOrLogOutErr != nil {
			return errors.Wrap(waitForAutoLockOrLogOutErr, "failed to wait for auto lock or log out")
		}
		return nil
	}
}

func (s *SmartCard) automaticActionOnSmartCardRemoval(option securityPolicyOption) action.Action {
	return action.Combine("automatic action on smart card removal",
		s.setSecurityTokenRemoval(option),
		s.ReAuthenticationOnline(),
		s.waitForAutoLockOrLogOut(option),
		s.insertSecondSmartCard(),
		s.signIn(),
		s.setCorrectPIN(),
	)
}

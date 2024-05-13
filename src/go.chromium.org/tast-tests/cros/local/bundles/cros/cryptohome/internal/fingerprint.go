// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func promptWithAsterisks(ctx context.Context, promptString string) {
	// This will look like:
	// *******************
	// * <prompt string> *
	// *******************
	promptString = "* " + promptString + " *"
	asterisks := strings.Repeat("*", len(promptString))
	testing.ContextLog(ctx, asterisks)
	testing.ContextLog(ctx, promptString)
	testing.ContextLog(ctx, asterisks)
}

// PromptFingerTouch prints the finger touch prompt.
func PromptFingerTouch(ctx context.Context, fingerName string) {
	promptWithAsterisks(ctx, "Please press your "+fingerName)
}

// PromptFingerEnroll prints the finger enroll prompt.
func PromptFingerEnroll(ctx context.Context, fingerName string) {
	promptWithAsterisks(ctx, "Please enroll your "+fingerName)
}

// PromptFingerMatch prints the finger match prompt.
func PromptFingerMatch(ctx context.Context, fingerName string) {
	promptWithAsterisks(ctx, "Please repeatedly press your "+fingerName)
}

// PromptFingerLift prints the finger lift prompt.
func PromptFingerLift(ctx context.Context) {
	promptWithAsterisks(ctx, "Please lift your finger")
	// GoBigSleepLint: we can't detect finger up event, so sleep 2 seconds and
	// assume the user has lifted their finger.
	testing.Sleep(ctx, 2*time.Second)
}

// EnrollFinger enrolls a new finger as a cryptohome fingerprint auth factor.
func EnrollFinger(ctx context.Context, client *hwsec.CryptohomeClient, authSessionID, fingerLabel, fingerName string) error {
	// Fingerprint enrollment might take up to 10 touches. Reserve 30 seconds.
	ctxWatcher, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	watcher, err := cryptohome.NewFingerprintEnrollmentWatcher(ctxWatcher)
	if err != nil {
		return errors.Wrap(err, "failed to create FingerprintEnrollmentWatcher")
	}
	defer watcher.Close(ctx)
	if _, err := client.PrepareAddFpAuthFactor(ctx, authSessionID); err != nil {
		return errors.Wrap(err, "failed to prepare fingerprint auth factor for add")
	}
	defer client.TerminateFpAuthFactor(ctx, authSessionID)

	PromptFingerTouch(ctx, fingerName)
	for {
		select {
		case sig, ok := <-watcher.Signals:
			if !ok {
				return errors.New("enrollment signal channel closed unexpectedly")
			}
			showEnrollmentProgress(ctx, *sig)
			if sig.Done {
				if err := client.AddFingerprintAuthFactor(ctx, authSessionID, fingerLabel); err != nil {
					return errors.Wrap(err, "failed to add fingerprint auth factor")
				}
				return nil
			}
			// There's a special error code that signals the enrollment session can't be continued.
			if sig.ScanResult == uda.FingerprintScanResult_FINGERPRINT_SCAN_RESULT_FATAL_ERROR {
				return errors.New("fingerprint enrollment failed with internal error")
			}
			PromptFingerTouch(ctx, fingerName)
		case <-ctxWatcher.Done():
			return errors.New("fingerprint enrollment timed out")
		}
	}
}

func showEnrollmentProgress(ctx context.Context, sig cryptohome.FingerprintEnrollmentSignal) {
	var scanStatusString string
	if sig.ScanResult == uda.FingerprintScanResult_FINGERPRINT_SCAN_RESULT_SUCCESS {
		scanStatusString = "Scan success"
	} else {
		// Note that the failure might just be a hint, and percent-complete might still increase
		// even if the scan status isn't success.
		scanStatusString = fmt.Sprintf("Scan failed: %v", sig.ScanResult)
	}
	testing.ContextLogf(ctx, "%s, progress: %v%%", scanStatusString, sig.PercentComplete)
}

// MatchFinger authenticates using a cryptohome fingerprint auth factor.
func MatchFinger(ctx context.Context, client *hwsec.CryptohomeClient, authSessionID string, fingerLabels []string, fingerName string) error {
	// Fingerprint authentication might take up to 5 touches due to false negatives (5th touch will lock it out anyway).
	// Reserve 5 seconds for each touch, 25 seconds in total.
	ctxWatcher, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	watcher, err := cryptohome.NewFingerprintAuthenticationWatcher(ctxWatcher)
	if err != nil {
		return errors.Wrap(err, "failed to create FingerprintAuthenticationWatcher")
	}
	defer watcher.Close(ctx)
	if _, err := client.PrepareAuthFpAuthFactor(ctx, authSessionID); err != nil {
		return errors.Wrap(err, "failed to prepare fingerprint auth factor for auth")
	}
	defer client.TerminateFpAuthFactor(ctx, authSessionID)

	PromptFingerTouch(ctx, fingerName)
	for {
		select {
		case sig, ok := <-watcher.Signals:
			if !ok {
				return errors.New("authentication signal channel closed unexpectedly")
			}
			// Scan should always succeed if there's no internal errors, as match is a later step.
			if sig.ScanResult != uda.FingerprintScanResult_FINGERPRINT_SCAN_RESULT_SUCCESS {
				return errors.New("fingerprint authentication failed with internal error")
			}
			if reply, err := client.AuthenticateFingerprintAuthFactor(ctx, authSessionID, fingerLabels); err != nil {
				if reply.ErrorInfo.PrimaryAction != uda.PrimaryAction_PRIMARY_INCORRECT_AUTH {
					// This error isn't retryable, return error.
					return errors.Wrap(err, "failed to authenticate fingerprint auth factor")
				}
				testing.ContextLog(ctx, "Fingerprint auth failed, please retry")
			} else {
				return nil
			}
			PromptFingerTouch(ctx, fingerName)
		case <-ctxWatcher.Done():
			return errors.New("fingerprint authentication timed out")
		}
	}
}

// MatchWrongFingerUntilLockout authenticates using a cryptohome fingerprint auth factor,
// but asks the user to press a wrong finger. It should fail repeatedly until the factor
// is locked out.
func MatchWrongFingerUntilLockout(ctx context.Context, client *hwsec.CryptohomeClient, authSessionID string, fingerLabels []string, wrongFingerName string) error {
	const (
		lockoutWrongAttempts = 5
	)

	// Fingerprint authentication takes 5 touches to get locked.
	// Reserve 5 seconds for each touch, 25 seconds in total.
	ctxWatcher, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	watcher, err := cryptohome.NewFingerprintAuthenticationWatcher(ctxWatcher)
	if err != nil {
		return errors.Wrap(err, "failed to create FingerprintAuthenticationWatcher")
	}
	defer watcher.Close(ctx)
	if _, err := client.PrepareAuthFpAuthFactor(ctx, authSessionID); err != nil {
		return errors.Wrap(err, "failed to prepare fingerprint auth factor for auth")
	}
	defer client.TerminateFpAuthFactor(ctx, authSessionID)

	PromptFingerTouch(ctx, wrongFingerName)
	for i := 1; i <= lockoutWrongAttempts; i++ {
		select {
		case sig, ok := <-watcher.Signals:
			if !ok {
				return errors.New("authentication signal channel closed unexpectedly")
			}
			// Scan should always succeed if there's no internal errors, as match is a later step.
			if sig.ScanResult != uda.FingerprintScanResult_FINGERPRINT_SCAN_RESULT_SUCCESS {
				return errors.New("fingerprint authentication failed with internal error")
			}
			if reply, err := client.AuthenticateFingerprintAuthFactor(ctx, authSessionID, fingerLabels); err != nil {
				var expectedAction uda.PrimaryAction
				if i == lockoutWrongAttempts {
					expectedAction = uda.PrimaryAction_PRIMARY_FACTOR_LOCKED_OUT
				} else {
					expectedAction = uda.PrimaryAction_PRIMARY_INCORRECT_AUTH
				}
				if reply.ErrorInfo.PrimaryAction != expectedAction {
					return errors.Wrapf(err, "authenticate fingerprint did not fail with primary action %v", expectedAction)
				}
				testing.ContextLog(ctx, "Fingerprint auth failed as expected")
			} else {
				return errors.New("fingerprint authentication succeeded with a wrong finger")
			}
			PromptFingerTouch(ctx, wrongFingerName)
		case <-ctxWatcher.Done():
			return errors.New("fingerprint authentication timed out")
		}
	}
	return nil
}

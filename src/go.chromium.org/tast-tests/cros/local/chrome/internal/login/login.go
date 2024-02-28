// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package login implements logging in to a Chrome user session.
package login

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/internal/config"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/driver"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

const oobePrefix = "chrome://oobe"
const rmaPrefix = "chrome://shimless-rma"

// Use a low polling interval while waiting for conditions during login, as this code is shared by many tests.
var pollOpts = &testing.PollOptions{Interval: 10 * time.Millisecond}

// ErrNeedNewSession is returned by LogIn if a caller should create a new
// session due to Chrome restart.
var ErrNeedNewSession = errors.New("Chrome restarted; need a new session")

// LogIn performs a user or guest login based on the loginMode.
// This function may restart Chrome and make an existing session unavailable,
// in which case errNeedNewSession is returned.
// Also performs enterprise enrollment before login when requested.
func LogIn(ctx context.Context, cfg *config.Config, sess *driver.Session) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	switch cfg.EnrollMode() {
	case config.NoEnroll:
		// Nothing to do.
	case config.FakeEnroll:
		if err := performFakeEnrollment(ctx, cfg, sess); err != nil {
			return err
		}
	case config.GAIAEnroll, config.SAMLTestIDPEnroll:
		if err := performGAIAEnrollment(ctx, cfg, sess); err != nil {
			return err
		}
	case config.ZeroTouchEnroll:
		if err := performZeroTouchEnrollment(ctx, cfg, sess); err != nil {
			return err
		}
	default:
		return errors.Errorf("unknown enrollment mode: %v", cfg.EnrollMode())
	}

	switch cfg.LoginMode() {
	case config.NoLogin:
		return nil
	case config.FakeLogin, config.GAIALogin, config.SAMLLogin:
		signals, err := cryptohome.NewLoginSignals(ctx)
		if err != nil {
			testing.ContextLog(ctx, "Failed to create login signals")
		} else {
			defer func(ctx context.Context) {
				if err := signals.Close(cleanupCtx); err != nil {
					testing.ContextLog(ctx, "Failed to cleanup signals: ", err)
				}
			}(cleanupCtx)
		}

		if err := loginUser(ctx, cfg, sess); err != nil {
			return err
		}

		if cfg.LoginMode() == config.SAMLLogin {
			return nil
		}

		if err := FinishUserLogin(ctx, cfg, sess, signals); err != nil {
			return errors.Wrap(err, "failed to finish user login")
		}

		return nil
	case config.GuestLogin:
		if err := logInAsGuest(ctx, cfg, sess); err != nil {
			return err
		}
		// logInAsGuest restarted Chrome. Let the caller know that they
		// need to recreate a session.
		return ErrNeedNewSession
	default:
		return errors.Errorf("unknown login mode: %v", cfg.LoginMode())
	}
}

// FinishUserLogin handles the necessary steps after a successful login.
func FinishUserLogin(ctx context.Context, cfg *config.Config, sess *driver.Session, signals *cryptohome.LoginSignals) error {
	if cfg.WaitForCryptohome() {
		if err := waitForCryptohome(ctx, cfg); err != nil {
			return errors.Wrapf(err, "waiting for cryptohome failed, %s", signals.ErrorMessage(ctx))
		}

	}

	if cfg.SkipOOBEAfterLogin() {
		ctx, st := timing.Start(ctx, "wait_for_oobe_dismiss")
		err := WaitForOOBEConnectionToBeDismissed(ctx, sess)
		st.End()
		if err != nil {
			return errors.Wrap(err, "waiting for OOBE to be dismissed failed")
		}
	}

	if cfg.RemoveNotification() {
		if err := removeNotifications(ctx, sess); err != nil {
			return errors.Wrap(err, "failed to remove notifications")
		}
	}

	return nil
}

// WaitForOOBEConnection establishes a connection to an OOBE page.
func WaitForOOBEConnection(ctx context.Context, sess *driver.Session) (*driver.Conn, error) {
	return WaitForOOBEConnectionWithPrefix(ctx, sess, oobePrefix)
}

// WaitForOOBEConnectionToBeDismissed waits for the OOBE page to be dismissed.
func WaitForOOBEConnectionToBeDismissed(ctx context.Context, sess *driver.Session) error {
	return waitForPageWithPrefixToBeDismissed(ctx, sess, oobePrefix)
}

// WaitForOOBEConnectionWithPrefix establishes a connection to the OOBE page matching the specified prefix.
func WaitForOOBEConnectionWithPrefix(ctx context.Context, sess *driver.Session, prefix string) (*driver.Conn, error) {
	testing.ContextLog(ctx, "Finding OOBE DevTools target")
	ctx, st := timing.Start(ctx, "wait_for_oobe")
	defer st.End()

	conn, err := waitForConnectionWithPrefix(ctx, sess, prefix)
	if err != nil {
		return nil, err
	}

	// Cribbed from telemetry/internal/backends/chrome/cros_browser_backend.py in Catapult.
	testing.ContextLog(ctx, "Waiting for OOBE")
	if err = conn.WaitForExpr(ctx, "typeof Oobe == 'function' && Oobe.readyForTesting"); err != nil {
		return nil, errors.Wrap(sess.Watcher().ReplaceErr(err), "OOBE didn't show up (Oobe.readyForTesting not found)")
	}
	if err = conn.WaitForExpr(ctx, "typeof OobeAPI == 'object'"); err != nil {
		return nil, errors.Wrap(sess.Watcher().ReplaceErr(err), "OOBE didn't show up (OobeAPI not found)")
	}

	connToRet := conn
	conn = nil
	return connToRet, nil
}

// WaitForRMAConnection establishes a connection to the RMA dialog.
func WaitForRMAConnection(ctx context.Context, sess *driver.Session) (*driver.Conn, error) {
	testing.ContextLog(ctx, "Finding RMA DevTools target")
	ctx, st := timing.Start(ctx, "wait_for_rma")
	defer st.End()

	return waitForConnectionWithPrefix(ctx, sess, rmaPrefix)
}

func waitForConnectionWithPrefix(ctx context.Context, sess *driver.Session, prefix string) (*driver.Conn, error) {
	var target *driver.Target
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		if target, err = getFirstTargetWithPrefix(ctx, sess, prefix); err != nil {
			return err
		} else if target == nil {
			return errors.Errorf("no %s target", prefix)
		}
		return nil
	}, pollOpts); err != nil {
		return nil, errors.Wrap(sess.Watcher().ReplaceErr(err), "RMA target not found")
	}

	conn, err := sess.NewConnForTarget(ctx, driver.MatchTargetID(target.TargetID))
	if err != nil {
		return nil, err
	}
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	connToRet := conn
	conn = nil
	return connToRet, nil
}

// getFirstTargetWithPrefix returns the first OOBE-related DevTools target matching a specified prefix.
// Returns nil if no matching target is found.
func getFirstTargetWithPrefix(ctx context.Context, sess *driver.Session, urlPrefix string) (*driver.Target, error) {
	targets, err := sess.FindTargets(ctx, func(t *driver.Target) bool {
		return strings.HasPrefix(t.URL, urlPrefix)
	})
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}
	return targets[0], nil
}

// waitForPageWithPrefixToBeDismissed waits for a OOBE page with a given prefix to disappear.
func waitForPageWithPrefixToBeDismissed(ctx context.Context, sess *driver.Session, urlPrefix string) error {
	testing.ContextLogf(ctx, "Waiting for OOBE %s to be dismissed", urlPrefix)
	errStillExists := errors.Errorf("%s target still exists", urlPrefix)

	// Save 5 seconds to try to get OOBE screen details in case of a failure.
	pollCtx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	err := testing.Poll(pollCtx, func(ctx context.Context) error {
		if t, err := getFirstTargetWithPrefix(ctx, sess, urlPrefix); err != nil {
			// This is likely a Chrome crash. So there's no chance that
			// waiting for the dismiss succeeds later. Quit the polling now.
			return testing.PollBreak(err)
		} else if t != nil {
			return errStillExists
		}
		return nil
	}, pollOpts)

	if err != nil {
		// If we are still on an OOBE page, try to get some details.
		if errors.Is(err, errStillExists) {
			// If the call fails, we fall back to the original error message.
			if name, step, screenErr := currentOOBEScreenDetails(ctx, sess); screenErr != nil {
				testing.ContextLog(ctx, "Failed to get OOBE screen details: ", screenErr)
			} else if title, subtitle, dialogueErr := currentOOBEDialogueDetails(ctx, sess); dialogueErr != nil {
				return errors.Wrapf(sess.Watcher().ReplaceErr(err), "OOBE not dismissed, it is on screen %q, step %q, DialogueErr: %q", name, step, dialogueErr)
			} else {
				testing.ContextLogf(ctx, "OOBE Dialogue subtitle: %q", subtitle)
				return errors.Wrapf(sess.Watcher().ReplaceErr(err), "OOBE not dismissed, it is on screen %q, step %q, title: %q", name, step, title)
			}
		}

		return errors.Wrap(sess.Watcher().ReplaceErr(err), "OOBE not dismissed")
	}

	return nil
}

// currentOOBEScreenDetails returns with the name and step name of the current OOBE screen.
func currentOOBEScreenDetails(ctx context.Context, sess *driver.Session) (string, string, error) {
	conn, err := WaitForOOBEConnection(ctx, sess)
	if err != nil {
		return "", "", err
	}

	var screenName string
	var screenStep string

	if err := conn.Eval(ctx, "OobeAPI.getCurrentScreenName()", &screenName); err != nil {
		return "", "", errors.Wrap(err, "failed to get OOBE screen name")
	}

	if err := conn.Eval(ctx, "OobeAPI.getCurrentScreenStep()", &screenStep); err != nil {
		return "", "", errors.Wrapf(err, "failed to get OOBE step on screen %q", screenName)
	}

	return screenName, screenStep, nil
}

// currentOOBEDialogueDetails returns with the title and subtitle of the current OOBE dialogue.
func currentOOBEDialogueDetails(ctx context.Context, sess *driver.Session) (string, string, error) {
	conn, err := WaitForOOBEConnection(ctx, sess)
	if err != nil {
		return "", "", err
	}

	var titleText string
	var subtitleText string

	if err := conn.Eval(ctx, "OobeAPI.getOobeActiveDialogTitleText()", &titleText); err != nil {
		return "", "", errors.Wrap(err, "failed to get OOBE Dialog title")
	}

	if err := conn.Eval(ctx, "OobeAPI.getOobeActiveDialogSubtitleText()", &subtitleText); err != nil {
		return "", "", errors.Wrap(err, "failed to get OOBE Dialog subtitle")
	}

	return titleText, subtitleText, nil
}

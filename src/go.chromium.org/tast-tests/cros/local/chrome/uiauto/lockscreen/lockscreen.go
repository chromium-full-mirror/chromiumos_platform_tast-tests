// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package lockscreen is used to get information about the lock screen directly through the UI.
package lockscreen

import (
	"context"
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const uiTimeout = 10 * time.Second

const authIconViewClassName = "AuthIconView"

var authErrorRegex = regexp.MustCompile(`Your.* password couldn't be verified. Try again.*`)

// AuthErrorFinder is the finder for the authentication error shown on the first failure.
var AuthErrorFinder = nodewith.Role(role.AlertDialog).NameRegex(authErrorRegex)

var consecutiveAuthErrorRegex = regexp.MustCompile(`Your.* password still couldn't be verified.*`)

// ConsecutiveAuthErrorFinder is the finder for the authentication error shown on the consecutive failures.
var ConsecutiveAuthErrorFinder = nodewith.Role(role.AlertDialog).NameRegex(consecutiveAuthErrorRegex)

// SmartLockArrowButtonFinder is the finder for the button that needs to be clicked to complete authentication with Smart Lock.
var SmartLockArrowButtonFinder = nodewith.NameContaining("Unlocked by your phone. Tap or click to enter.").HasClass("ArrowButtonView")

// SimplePinOrPasswordFieldFinder is like PINFieldFinder, but doesn't check the name attribute so that username doesn't
// need to be passed in and it's more convenient to use.
var SimplePinOrPasswordFieldFinder = nodewith.Role(role.TextField).Attribute("placeholder", "PIN or password")

// PinInputFieldFinder finds the node that displays the entered PIN when autosubmit is
// enabled.
var PinInputFieldFinder = nodewith.HasClass("LoginPinInputView")

// recoverUserFinder is the finder for the user recovery button.
var recoverUserFinder = nodewith.Role(role.Button).Name("Recover user")

// State contains the state returned by chrome.autotestPrivate.loginStatus,
// corresponding to 'LoginStatusDict' as defined in autotest_private.idl.
type State struct {
	LoggedIn               bool   `json:"isLoggedIn"`
	Owner                  bool   `json:"isOwner"`
	Locked                 bool   `json:"isScreenLocked"`
	WallpaperAnimating     bool   `json:"isLockscreenWallpaperAnimating"`
	ReadyForPassword       bool   `json:"isReadyForPassword"` // Login screen may not be ready to receive a password, even if this is true (crbug/1109381)
	RegularUser            bool   `json:"isRegularUser"`
	Guest                  bool   `json:"isGuest"`
	Kiosk                  bool   `json:"isKiosk"`
	AreAllUserImagesLoaded bool   `json:"areAllUserImagesLoaded"`
	Email                  string `json:"email"`
	DisplayEmail           string `json:"displayEmail"`
	UserImage              string `json:"userImage"`
	HasValidOauth2Token    bool   `json:"hasValidOauth2Token"`
}

// GetState returns the login status information from chrome.autotestPrivate.loginStatus
func GetState(ctx context.Context, tconn *chrome.TestConn) (State, error) {
	var st State
	if err := tconn.Call(ctx, &st, `tast.promisify(chrome.autotestPrivate.loginStatus)`); err != nil {
		return st, errors.Wrap(err, "failed calling chrome.autotestPrivate.loginStatus")
	}
	return st, nil
}

// WaitState repeatedly calls GetState and passes the returned state to check
// until it returns true or timeout is reached. The last-seen state is returned.
func WaitState(ctx context.Context, tconn *chrome.TestConn, check func(st State) bool, timeout time.Duration) (State, error) {
	var st State
	err := testing.Poll(ctx, func(ctx context.Context) error {
		var e error
		st, e = GetState(ctx, tconn)
		if e != nil {
			return testing.PollBreak(e)
		}
		if !check(st) {
			return errors.New("wrong status")
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
	return st, err
}

// WaitForLoggedIn is a wrapper around WaitState to wait for the user to be logged in.
func WaitForLoggedIn(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration) error {
	if st, err := WaitState(ctx, tconn, func(st State) bool { return st.LoggedIn }, timeout); err != nil {
		return errors.Wrapf(err, "waiting for logged in state failed: (last status %+v)", st)
	}
	return nil
}

// WaitForPasswordEntry is a wrapper around WaitState to wait for the screen to
// be ready for password, and for the user images to be loaded.
func WaitForPasswordEntry(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration) error {
	if st, err := WaitState(ctx, tconn, func(st State) bool {
		return st.ReadyForPassword && st.AreAllUserImagesLoaded
	}, timeout); err != nil {
		return errors.Wrapf(err, "waiting to be ready for password entry failed: (last status %+v)", st)
	}
	return nil
}

// PasswordFieldFinder generates Finder for the password field.
// The password field node can be uniquely identified by its name attribute, which includes the username,
// such as "Password for username@gmail.com". The Finder will find the node whose name matches the regex
// /Password for <username>/, so the domain can be omitted, or the username argument can be an empty
// string to find the first password field in the hierarchy.
func PasswordFieldFinder(username string) (*nodewith.Finder, error) {
	r, err := regexp.Compile(fmt.Sprintf("Password for %v", username))
	if err != nil {
		return nil, errors.Wrap(err, "failed to compile regexp for name attribute")
	}
	return nodewith.Role(role.TextField).Attribute("name", r).Attribute("placeholder", "Password"), nil
}

// WaitForPasswordField waits for the password text field for a given user pod to appear in the UI.
func WaitForPasswordField(ctx context.Context, tconn *chrome.TestConn, username string, timeout time.Duration) error {
	finder, err := PasswordFieldFinder(username)
	if err != nil {
		return err
	}
	return uiauto.New(tconn).WithTimeout(timeout).WaitUntilExists(finder)(ctx)
}

// WaitForAuthError waits for the login error bubble that password or pin was not correct.
func WaitForAuthError(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration) error {
	return uiauto.New(tconn).WithTimeout(timeout).WaitUntilExists(AuthErrorFinder)(ctx)
}

func typePasswordAndVerify(ctx context.Context, tconn *chrome.TestConn, username, password string, submit bool, kb *input.KeyboardEventWriter) error {
	if st, err := WaitState(ctx, tconn, func(st State) bool { return st.ReadyForPassword }, 3*uiTimeout); err != nil {
		return errors.Wrapf(err, "failed to wait for screen to be ready for password (last status %+v)", st)
	}
	field, err := PasswordFieldFinder(username)
	if err != nil {
		return err
	}
	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(uiTimeout).WaitUntilExists(field)(ctx); err != nil {
		return errors.Wrap(err, "failed to find password box")
	}
	if err := ui.LeftClick(field)(ctx); err != nil {
		return errors.Wrap(err, "failed to click password box")
	}
	// Wait for the field to be focused before entering the password.
	if err := ui.WaitUntilExists(field.Focused())(ctx); err != nil {
		return errors.Wrap(err, "password field not focused yet")
	}
	if err := kb.Type(ctx, password); err != nil {
		return errors.Wrap(err, "failed to type password")
	}
	enteredPass, err := UserPassword(ctx, tconn, username, false /*pin*/)
	if err != nil {
		return errors.Wrap(err, "failed to read password")
	}
	// We have to use RuneCount instead of len because password is concealed and all characters are
	// replaced by the bullet character, which is more than one byte in utf8.
	enteredLen := utf8.RuneCountInString(enteredPass.Value)
	// Verify whether the entered password is correct.
	if len(password) != enteredLen {
		if err := ShowPassword(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to click the Show password button for verification")
		}
		enteredRevealedPass, err := UserPassword(ctx, tconn, username, false /*pin*/)
		if err != nil {
			return errors.Wrap(err, "failed to read password for verification")
		}
		return errors.Errorf("failed to enter password, got %s, want %s", enteredRevealedPass.Value, password)
	}
	if !submit {
		return nil
	}

	if err := kb.Type(ctx, "\n"); err != nil {
		return errors.Wrap(err, "failed to submit password")
	}
	return nil
}

// TypePassword enters the given password (without submitting). Refer to PasswordFieldFinder for username options.
// It doesn't make any assumptions about the password being correct, so callers should verify the login/lock state afterwards.
func TypePassword(ctx context.Context, tconn *chrome.TestConn, username, password string, kb *input.KeyboardEventWriter) error {
	return typePasswordAndVerify(ctx, tconn, username, password, false /*submit*/, kb)
}

// EnterPassword types password with carriage return at the end.
func EnterPassword(ctx context.Context, tconn *chrome.TestConn, username, password string, kb *input.KeyboardEventWriter) error {
	return typePasswordAndVerify(ctx, tconn, username, password, true /*submit*/, kb)
}

// Lock locks the screen.
func Lock(ctx context.Context, tconn *chrome.TestConn) error {
	if err := tconn.Eval(ctx, `chrome.autotestPrivate.lockScreen()`, nil); err != nil {
		return errors.Wrap(err, "failed calling chrome.autotestPrivate.lockScreen")
	}
	if st, err := WaitState(ctx, tconn, func(st State) bool { return st.Locked && st.ReadyForPassword }, 3*uiTimeout); err != nil {
		return errors.Wrapf(err, "failed to wait for screen to be locked (last status %+v)", st)
	}
	return nil
}

// EnterPIN enters the specified PIN.
func EnterPIN(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, PIN string) error {
	ui := uiauto.New(tconn)

	// Wait until one of the "PIN or password" field or the PIN-only input field appears and accepts
	// input (i.e. not disabled, not read only).

	// For PIN only input, each entered digit is displayed in a separate input field. This finder
	// matches the input field holding the first digit.
	digitFieldFinder := nodewith.Role(role.TextField).Ancestor(PinInputFieldFinder).First()

	if err := ui.WithTimeout(uiTimeout).WaitUntilAnyExists(SimplePinOrPasswordFieldFinder, digitFieldFinder)(ctx); err != nil {
		return errors.Wrap(err, "failed to find PIN input field")
	}
	inputFinder, err := ui.FindAnyExists(ctx, SimplePinOrPasswordFieldFinder, digitFieldFinder)
	if err != nil {
		return errors.Wrap(err, "failed to find PIN input field again")
	}

	if err := ui.WithTimeout(uiTimeout).WaitForRestriction(inputFinder, restriction.None)(ctx); err != nil {
		return errors.Wrap(err, "failed to find PIN input field unrestricted")
	}

	// If we can't find the PIN pad, we click the input field and enter the PIN via the keyboard. If
	// we do find the PIN pad, we use the PIN pad to enter the PIN.
	if err := ui.WithTimeout(uiTimeout).WaitUntilExists(nodewith.HasClass("LoginPinView"))(ctx); err != nil {
		if err := ui.WithTimeout(uiTimeout).WaitUntilExists(SimplePinOrPasswordFieldFinder)(ctx); err != nil {
			return errors.Wrap(err, "failed to find PIN or password box")
		}
		if err := ui.LeftClick(SimplePinOrPasswordFieldFinder)(ctx); err != nil {
			return errors.Wrap(err, "failed to click PIN or password box")
		}
		// Wait for the field to be focused before entering PIN.
		if err := ui.WaitUntilExists(SimplePinOrPasswordFieldFinder.Focused())(ctx); err != nil {
			return errors.Wrap(err, "PIN or password field not focused yet")
		}
		if err := kb.Type(ctx, PIN); err != nil {
			return errors.Wrap(err, "failed to type PIN")
		}
	} else {
		//return errors.New("found LoginPinView");
		for i, d := range PIN {
			button := nodewith.Role(role.Button).Name(string(d))
			if err := ui.WithTimeout(uiTimeout).DoDefault(button)(ctx); err != nil {
				return errors.Wrapf(err, "failed to press %q button (Digit %v of PIN)", d, i)
			}
		}
	}
	return nil
}

// WaitUntilPinPadExists waits until the PIN pad is present.
func WaitUntilPinPadExists(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	return ui.WaitUntilExists(nodewith.HasClass("LoginPinView"))(ctx)
}

// WaitUntilPinPadGone waits until the PIN pad is gone.
func WaitUntilPinPadGone(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	return ui.WaitUntilGone(nodewith.HasClass("LoginPinView"))(ctx)
}

// SubmitPINOrPassword submits the entered PIN.
func SubmitPINOrPassword(ctx context.Context, tconn *chrome.TestConn) error {
	return uiauto.New(tconn).WithTimeout(uiTimeout).LeftClick(SubmitButton)(ctx)
}

// ClickSmartLockArrowButton clicks the arrow button to login with Smart Lock.
func ClickSmartLockArrowButton(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	return ui.WithTimeout(uiTimeout).DoDefault(SmartLockArrowButtonFinder)(ctx)
}

// WaitForSmartLockReady waits for UI signal that the chromebook is ready to be unlocked by Smart Lock.
func WaitForSmartLockReady(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(SmartLockArrowButtonFinder)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for Smart Lock UI to indicate it is ready to unlock")
	}
	return nil
}

// WaitForSmartLockVisible waits for UI signal that Smart Lock is an available auth factor.
func WaitForSmartLockVisible(ctx context.Context, expectVisible bool, tconn *chrome.TestConn) error {
	finder := nodewith.HasClass(authIconViewClassName)
	ui := uiauto.New(tconn)
	if expectVisible {
		if err := ui.WaitUntilExists(finder)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for Smart Lock UI to show up")
		}
	} else {
		if err := ui.EnsureGoneFor(finder, 5*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to ensure Smart Lock UI gone for 5 sec")
		}
	}
	return nil
}

// ShowPassword clicks the "Show password" button.
func ShowPassword(ctx context.Context, tconn *chrome.TestConn) error {
	return uiauto.New(tconn).WithTimeout(uiTimeout).LeftClick(ShowPasswordButton)(ctx)
}

// HidePassword clicks the "Hide password" button.
func HidePassword(ctx context.Context, tconn *chrome.TestConn) error {
	return uiauto.New(tconn).WithTimeout(uiTimeout).LeftClick(HidePasswordButton)(ctx)
}

// SwitchToPassword clicks the "Switch to password" button which appears only when PIN autosubmit is enabled.
func SwitchToPassword(ctx context.Context, tconn *chrome.TestConn) error {
	return uiauto.New(tconn).WithTimeout(uiTimeout).LeftClick(SwitchToPasswordButton)(ctx)
}

// PINFieldFinder generates Finder for the "PIN or password" field.
func PINFieldFinder(username string) (*nodewith.Finder, error) {
	r := regexp.MustCompile(fmt.Sprintf("Password for %v", username))
	return nodewith.Role(role.TextField).Attribute("name", r).Attribute("placeholder", "PIN or password"), nil
}

// UserPassword searches the PIN / Password field for a given user pod and returns the corresponding node.
func UserPassword(ctx context.Context, tconn *chrome.TestConn, username string, pin bool) (*uiauto.NodeInfo, error) {
	var field *nodewith.Finder
	var err error
	if pin {
		field, err = PINFieldFinder(username)
		if err != nil {
			return nil, errors.Wrap(err, "failed to find the node for \"PIN or password\" field")
		}
	} else {
		field, err = PasswordFieldFinder(username)
		if err != nil {
			return nil, errors.Wrap(err, "failed to find the Password node")
		}
	}
	return uiauto.New(tconn).Info(ctx, field)
}

// Unlock unlocks the screen, assuming that PIN / Password has already been entered by the user.
func Unlock(ctx context.Context, tconn *chrome.TestConn) error {
	if err := SubmitPINOrPassword(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to click the Submit button")
	}
	if st, err := WaitState(ctx, tconn, func(st State) bool { return st.LoggedIn }, 3*uiTimeout); err != nil {
		return errors.Wrapf(err, "failed waiting to log in (last state: %+v)", st)
	}
	return nil
}

// UnlockWithPassword unlocks the screen by typing the given password, assuming that the screen is locked.
func UnlockWithPassword(ctx context.Context, tconn *chrome.TestConn, username, password string, kb *input.KeyboardEventWriter, lockTimeout, authTimeout time.Duration) error {
	if st, err := WaitState(ctx, tconn, func(st State) bool { return st.ReadyForPassword }, lockTimeout); err != nil {
		return errors.Wrapf(err, "failed waiting until lock screen is ready for password (last state: %+v)", st)
	}
	if err := EnterPassword(ctx, tconn, username, password, kb); err != nil {
		return errors.Wrap(err, "failed unlocking the screen")
	}
	if st, err := WaitState(ctx, tconn, func(st State) bool { return !st.Locked }, authTimeout); err != nil {
		return errors.Wrapf(err, "failed waiting to log in (last state: %+v)", st)
	}
	return nil
}

// ClickRecoverUser clicks the user recovery button on the login screen.
// The button must be already visible.
func ClickRecoverUser(ctx context.Context, tconn *chrome.TestConn) error {
	return uiauto.New(tconn).WithTimeout(uiTimeout).LeftClick(recoverUserFinder)(ctx)
}

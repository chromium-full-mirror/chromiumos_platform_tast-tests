// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Clipboard,
		Desc:     "Tests copying and pasting from Chrome to Android and vice versa",
		Contacts: []string{"arc-framework+tast@google.com", "yhanada@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Chrome Integration
		BugComponent: "b:537221",
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"clipboard.html", "clipboard_image.html"},
		Attr:         []string{"group:mainline", "group:release-health", "release-health_arc"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 1*time.Minute,
		Params: []testing.Param{
			{
				Name:              "vm",
				Fixture:           "arcBooted",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			}},
	})
}

// idPrefix is the prefix to all view IDs in the helper Android app.
const idPrefix = "org.chromium.arc.testapp.clipboard:id/"

// sleepTimeAfterCopy is a time to sleep after triggering a copy.
// It takes time for clipboard data to go between Chrome and Android,
// but there's no way to tell whether the data is actually passed between them
// without actually invoking paste action.
const sleepTimeAfterCopy = 2 * time.Second

// A copyFunc encapsulates a "copy" operation which is predecided (e.g. data to
// be copied and clipboard destination).
type copyFunc func(context.Context) error

// A copyFunc encapsulates a "paste" operation which is predecided (e.g.
// clipboard source and paste mechanism).
type pasteFunc func(context.Context) (string, error)

// bringChromeCopyPasteWindowToFront moves the Chrome window to the front.
func bringChromeCopyPasteWindowToFront(ctx context.Context, tconn *chrome.TestConn) error {
	const windowTitle = "Chrome - ClipboardTestPage"
	w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
		return w.WindowType == ash.WindowTypeBrowser && w.Title == windowTitle
	})
	if err != nil {
		return errors.Wrap(err, "failed to find ClipboardTest window")
	}
	return w.ActivateWindow(ctx, tconn)
}

// bringAndroidCopyPasteWindowToFront moves the Android test app to the front.
func bringAndroidCopyPasteWindowToFront(ctx context.Context, tconn *chrome.TestConn) error {
	_, err := ash.BringWindowToForeground(ctx, tconn, "ArcClipboardTest")
	return err
}

// prepareCopyInChrome sets up a copy operation with Chrome as the source
// clipboard.
// Due to the security reason (crbug.com/1334203), writing to the clipboard
// works only with a user gesture. That's why this function uses clicking and
// typing instead of running js on the page.
func prepareCopyInChrome(tconn *chrome.TestConn, uia *uiauto.Context, keyboard *input.KeyboardEventWriter, format, data, baseURL string) copyFunc {
	return func(ctx context.Context) error {
		if err := bringChromeCopyPasteWindowToFront(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to bring the Chrome window foreground")
		}

		dataBoxNode := nodewith.HasClass("data").Role(role.TextField).State(state.Editable, true).First()
		formatBoxNode := nodewith.HasClass("format").Role(role.TextField).State(state.Editable, true).First()
		copyButtonNode := nodewith.HasClass("copy").Role(role.Button).First()

		// Put the copying data to the field
		if err := uiauto.Combine("Copying the data to the text field",
			uia.WaitUntilExists(dataBoxNode.Visible()),
			uia.LeftClick(dataBoxNode),
			uia.WaitUntilExists(dataBoxNode.Focused()),
			keyboard.AccelAction("ctrl+a"),
			keyboard.TypeAction(data),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to type the data")
		}

		// Put the format info to the field
		if err := uiauto.Combine("Typing the format to the text field",
			uia.WaitUntilExists(formatBoxNode.Visible()),
			uia.LeftClick(formatBoxNode),
			uia.WaitUntilExists(formatBoxNode.Focused()),
			keyboard.AccelAction("ctrl+a"),
			keyboard.TypeAction(format),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to type the format")
		}

		// Click the copy button
		if err := uiauto.Combine("Clicking the copy button",
			uia.WaitUntilExists(copyButtonNode.Visible()),
			uia.LeftClick(copyButtonNode),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to click the copy button")
		}

		return nil
	}
}

// preparePasteInChrome sets up a paste operation with Chrome as the
// destination clipboard.
// For the security reason (crbug.com/1334203), reading from the clipboard
// works only after granting the permission. That's why this function uses clicking and
// typing instead of running js on the page.
func preparePasteInChrome(conn *chrome.Conn, tconn *chrome.TestConn, uia *uiauto.Context, keyboard *input.KeyboardEventWriter, format, baseURL string) pasteFunc {
	return func(ctx context.Context) (string, error) {
		if err := bringChromeCopyPasteWindowToFront(ctx, tconn); err != nil {
			return "", errors.Wrap(err, "failed to bring the Chrome window foreground")
		}

		formatBoxNode := nodewith.HasClass("format").Role(role.TextField).State(state.Editable, true).First()
		pasteButtonNode := nodewith.HasClass("paste").Role(role.Button).First()

		// Put the format info to the field
		if err := uiauto.Combine("Typing the format to the text field",
			uia.WaitUntilExists(formatBoxNode.Visible()),
			uia.LeftClick(formatBoxNode),
			uia.WaitUntilExists(formatBoxNode.Focused()),
			keyboard.AccelAction("ctrl+a"),
			keyboard.TypeAction(format),
		)(ctx); err != nil {
			return "", errors.Wrap(err, "failed to type the format")
		}

		// Click the paste button
		if err := uiauto.Combine("Clicking the paste button",
			uia.WaitUntilExists(pasteButtonNode.Visible()),
			uia.LeftClick(pasteButtonNode),
		)(ctx); err != nil {
			return "", errors.Wrap(err, "failed to click the paste button")
		}

		// Click the allow dialog if shown
		allowButtonNode := nodewith.HasClass("MdTextButton").Name("Allow").First()
		if err := uiauto.IfSuccessThen(
			uia.WithTimeout(5*time.Second).WaitUntilEnabled(allowButtonNode),
			uia.LeftClick(allowButtonNode),
		)(ctx); err != nil {
			return "", errors.Wrap(err, "failed to click the allow button")
		}

		var result string
		conn.Eval(ctx, `document.getElementById('data').value`, &result)

		return result, nil
	}
}

// prepareCopyInAndroid sets up a copy operation with Android as the source
// clipboard. The Android helper app we use during this test has a series of
// buttons which populate some views with text (hard-coded into the app); in
// order to execute a copy operation, we first have to click a button (the
// writeDataBtnID) to populate the right text and then we click the
// "copy_button". Since the text to copy is hard-coded into the android app,
// this helper ensures that the app and this test are in-sync with regards to
// the text that we expect to be copied, by checking that the provided
// viewIDForGetText contains the provided expected string.
func prepareCopyInAndroid(d *ui.Device, tconn *chrome.TestConn, writeDataBtnID, viewIDForGetText, expected string) copyFunc {
	const copyID = idPrefix + "copy_button"

	return func(ctx context.Context) error {
		if err := bringAndroidCopyPasteWindowToFront(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to bring the Android window foreground")
		}

		if err := d.Object(ui.ID(writeDataBtnID)).Click(ctx); err != nil {
			return errors.Wrap(err, "failed to set text in Android EditText")
		}
		if text, err := d.Object(ui.ID(viewIDForGetText)).GetText(ctx); err != nil {
			return errors.Wrap(err, "failed to obtain Android text")
		} else if text != expected {
			return errors.Errorf("failed to set up the content to be copied in Android: got %q; want %q", text, expected)
		}
		if err := d.Object(ui.ID(copyID)).Click(ctx); err != nil {
			return errors.Wrap(err, "failed to copy the text in Android")
		}
		return nil
	}
}

// preparePasteInAndroid sets up a paste operation with Android as the
// destination clipboard. Specifically: in the Android helper app, the
// "paste_button" is clicked and the contents of the provided view ID is
// returned via GetText.
func preparePasteInAndroid(d *ui.Device, tconn *chrome.TestConn, viewIDForGetText string) pasteFunc {
	const pasteID = idPrefix + "paste_button"

	return func(ctx context.Context) (string, error) {
		if err := bringAndroidCopyPasteWindowToFront(ctx, tconn); err != nil {
			return "", errors.Wrap(err, "failed to bring the Android window foreground")
		}

		if err := d.Object(ui.ID(pasteID)).Click(ctx); err != nil {
			return "", errors.Wrap(err, "failed to paste")
		}

		text, err := d.Object(ui.ID(viewIDForGetText)).GetText(ctx)
		if err != nil {
			return "", errors.Wrap(err, "failed to obtain pasted text")
		}
		return text, nil
	}
}

func testCopyImageFromChromeToAndroid(ctx context.Context, p *arc.PreData, tconn *chrome.TestConn, fs http.FileSystem) error {
	const (
		apk = "ArcClipboardTest.apk"
		pkg = "org.chromium.arc.testapp.clipboard"
		cls = "org.chromium.arc.testapp.clipboard.ClipboardActivity"

		titleID = idPrefix + "text_view"
		title   = "HTML tags goes here"

		textViewID = idPrefix + "text_view"
	)

	cr := p.Chrome
	a := p.ARC
	d := p.UIDevice

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(ctx)

	server := httptest.NewServer(http.FileServer(fs))
	defer server.Close()

	// Open the html with an image.
	conn, err := cr.NewConn(ctx, server.URL+"/clipboard_image.html")
	if err != nil {
		return errors.Wrap(err, "failed to open clipboard_image.html")
	}
	defer conn.Close()

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for the page loaded")
	}

	encodedImage := ""
	conn.Eval(ctx, "document.getElementById('image').getAttribute('src')", &encodedImage)

	if err := uiauto.Combine("copy all text from source website",
		keyboard.AccelAction("Ctrl+A"),
		keyboard.AccelAction("Ctrl+C"))(ctx); err != nil {
		return errors.Wrap(err, "failed to copy text from source browser")
	}

	conn.CloseTarget(ctx)

	act, err := arc.NewActivity(a, pkg, cls)
	if err != nil {
		return errors.Wrap(err, "failed to create a new activity")
	}
	defer act.Close(ctx)
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to start the activity")
	}
	defer act.Stop(ctx, tconn)

	if err := d.Object(ui.ID(titleID), ui.Text(title)).WaitForExists(ctx, 30*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for the app shown")
	}

	pasteAndroid := preparePasteInAndroid(d, tconn, textViewID)
	// Paste in Android.
	androidHTML, err := pasteAndroid(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to obtain pasted image")
	}

	// Verify the result.
	// Note: style attribute is added by Chrome before the image is copied to Android.
	re := regexp.MustCompile(`^<img id="image" src="(.+?)" style=".+?">$`)
	if m := re.FindStringSubmatch(androidHTML); m == nil {
		return errors.Wrapf(err, "failed to find pasted image in Android: got %q", androidHTML)
	} else if m[1] != encodedImage {
		return errors.Wrapf(err, "unexpected paste result: got %q; want %q", m[1], encodedImage)
	}

	return nil
}

func Clipboard(ctx context.Context, s *testing.State) {
	const (
		apk = "ArcClipboardTest.apk"
		pkg = "org.chromium.arc.testapp.clipboard"
		cls = "org.chromium.arc.testapp.clipboard.ClipboardActivity"

		titleID = idPrefix + "text_view"
		title   = "HTML tags goes here"

		editTextID = idPrefix + "edit_message"
		textViewID = idPrefix + "text_view"
	)

	p := s.FixtValue().(*arc.PreData)
	cr := p.Chrome
	a := p.ARC
	d := p.UIDevice

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	uia := uiauto.New(tconn)

	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	// Copy image from Chrome to Android.
	s.Run(ctx, "CopyImageFromChromeToAndroid", func(ctx context.Context, s *testing.State) {
		handler := func(msg string) {
			faillog.DumpUITreeWithScreenshotWithTestAPIOnError(ctx, s.OutDir(), s.HasError, tconn, "CopyImageFromChromeToAndroid")
		}
		s.AttachErrorHandlers(handler, handler)

		if err := testCopyImageFromChromeToAndroid(ctx, p, tconn, s.DataFileSystem()); err != nil {
			s.Fatal("Failed to verify copying an image from a browser to an app: ", err)
		}
	})

	s.Log("Starting app")
	act, err := arc.NewActivity(a, pkg, cls)
	if err != nil {
		s.Fatal("Failed to create a new activity: ", err)
	}
	defer act.Close(ctx)
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed to start the activity: ", err)
	}
	defer act.Stop(ctx, tconn)

	s.Log("Waiting for App showing up")
	if err := d.Object(ui.ID(titleID), ui.Text(title)).WaitForExists(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to wait for the app shown: ", err)
	}

	info, err := ash.GetARCAppWindowInfo(ctx, tconn, act.PackageName())
	if err != nil {
		s.Fatal("Failed to window info of the activity: ", err)
	}

	// Click the center of the activity from Chrome to generate the first mouse event, because
	// Wayland's set_selection should be associated with a valid serial number from an actual event.
	if err := mouse.Click(tconn, info.BoundsInRoot.CenterPoint(), mouse.LeftButton)(ctx); err != nil {
		s.Fatal("Failed to click the center of the app: ", err)
	}

	conn, err := cr.NewConn(ctx, server.URL+"/clipboard.html")
	if err != nil {
		s.Fatal("Failed to open the clipboard.html: ", err)
	}
	defer conn.Close()

	if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for the page loaded: ", err)
	}

	s.Run(ctx, "CopyHTMLFromChromeToAndroidWithObserver", func(ctx context.Context, s *testing.State) {
		handler := func(msg string) {
			faillog.DumpUITreeWithScreenshotWithTestAPIOnError(ctx, s.OutDir(), s.HasError, tconn, "CopyHTMLFromChromeToAndroidWithObserver")
		}
		s.AttachErrorHandlers(handler, handler)

		const (
			observerEnableID   = idPrefix + "enable_observer_button"
			observerDisableID  = idPrefix + "disable_observer_button"
			observerTextViewID = idPrefix + "observer_view"
			observerReady      = "Observer ready"
		)
		// Enable observer and wait for it to be ready to prevent a possible race.
		if err := bringAndroidCopyPasteWindowToFront(ctx, tconn); err != nil {
			s.Fatal("Failed to bring the Android window foreground: ", err)
		}
		if err := d.Object(ui.ID(observerEnableID)).Click(ctx); err != nil {
			s.Fatal("Failed to enable observer: ", err)
		}
		defer func() {
			if err := bringAndroidCopyPasteWindowToFront(ctx, tconn); err != nil {
				s.Error("Failed to bring the Android window foreground: ", err)
			}
			if err := d.Object(ui.ID(observerDisableID)).WaitForExists(ctx, 10*time.Second); err != nil {
				s.Error("Failed to wait for the disable button shown: ", err)
			}
			if err := d.Object(ui.ID(observerDisableID)).Click(ctx); err != nil {
				s.Error("Failed to disable observer: ", err)
			}
		}()
		if err := d.Object(ui.ID(observerTextViewID)).WaitForText(ctx, observerReady, 5*time.Second); err != nil {
			s.Fatal("Failed to wait for observer ready: ", err)
		}

		if err := bringChromeCopyPasteWindowToFront(ctx, tconn); err != nil {
			s.Error("Failed to bring the Android window foreground: ", err)
		}

		// Copy in Chrome, so the registered observer should paste the clipboard content in Android.
		const content = "<b>observer</b> should paste this"
		const newContent = "<html><head></head><body><b>observer</b> should paste this</body></html>"
		chromeCopy := prepareCopyInChrome(tconn, uia, keyboard, "text/html", content, server.URL)
		if err := chromeCopy(ctx); err != nil {
			s.Fatal("Failed to copy in Chrome: ", err)
		}

		// GoBigSleepLint - wait a bit to make sure clipboard data goes through the vm boundary.
		if err := testing.Sleep(ctx, sleepTimeAfterCopy); err != nil {
			s.Error("Failed to sleep: ", err)
		}

		// Paste and Verify the result.
		// TODO(crbug.com/1510998): Remove newContent once Chromium changes are submitted.
		if err := bringAndroidCopyPasteWindowToFront(ctx, tconn); err != nil {
			s.Fatal("Failed to bring the Android window foreground: ", err)
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if html, err := d.Object(ui.ID(textViewID)).GetText(ctx); err != nil {
				return err
			} else if html != content && html != newContent {
				return errors.Errorf("Observed clipboard is unexpected in Android: got %q; want %q", html, content)
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			s.Fatal("Failed to observe clipboard in Android: ", err)
		}
	})

	const (
		// clicking writeTextBtnID causes the "Test Text 1234" message to show up
		// in the editTextID.
		writeTextBtnID          = idPrefix + "write_text_button"
		expectedTextFromAndroid = "Test Text 1234"
		// clicking writeHTMLBtnID causes the following HTML to show up in the
		// textViewID.
		writeHTMLBtnID          = idPrefix + "write_html_button"
		expectedHTMLFromAndroid = `<p dir="ltr">test <b>HTML</b> 1234</p>`

		testTextFromChrome = "Text to be copied from Chrome to Android"
		testHTMLFromChrome = "<b>bold</b><i>italics</i>"
	)

	for _, row := range []struct {
		name string

		copyFunc       copyFunc
		pasteFunc      pasteFunc
		wantPastedData string
	}{{
		"CopyTextFromChromeToAndroid",
		prepareCopyInChrome(tconn, uia, keyboard, "text/plain", testTextFromChrome, server.URL),
		preparePasteInAndroid(d, tconn, editTextID),
		testTextFromChrome,
	}, {
		"CopyTextFromAndroidToChrome",
		prepareCopyInAndroid(d, tconn, writeTextBtnID, editTextID, expectedTextFromAndroid),
		preparePasteInChrome(conn, tconn, uia, keyboard, "text/plain", server.URL),
		expectedTextFromAndroid,
	}, {
		"CopyHTMLFromChromeToAndroid",
		prepareCopyInChrome(tconn, uia, keyboard, "text/plain", testHTMLFromChrome, server.URL),
		preparePasteInAndroid(d, tconn, textViewID),
		testHTMLFromChrome,
	}, {
		"CopyHTMLFromAndroidToChrome",
		prepareCopyInAndroid(d, tconn, writeHTMLBtnID, textViewID, expectedHTMLFromAndroid),
		preparePasteInChrome(conn, tconn, uia, keyboard, "text/html", server.URL),
		expectedHTMLFromAndroid,
	}} {
		s.Run(ctx, row.name, func(ctx context.Context, s *testing.State) {
			handler := func(msg string) {
				faillog.DumpUITreeWithScreenshotWithTestAPIOnError(ctx, s.OutDir(), s.HasError, tconn, row.name)
			}
			s.AttachErrorHandlers(handler, handler)

			start := time.Now()
			if err := row.copyFunc(ctx); err != nil {
				s.Fatal("Failed to copy: ", err)
			}

			// GoBigSleepLint - wait a bit to make sure clipboard data goes through the vm boundary.
			if err := testing.Sleep(ctx, sleepTimeAfterCopy); err != nil {
				s.Error("Failed to sleep: ", err)
			}

			// Rather than assuming the copy is effective ~immediately, we have to
			// poll because the latency of the operation is slower in newer
			// kernels (e.g. 5.4, at time of writing). See b/157615371 for
			// historical background.
			afterCopy := time.Now()
			attempt := 0
			err := testing.Poll(ctx, func(ctx context.Context) error {
				got, err := row.pasteFunc(ctx)
				if err != nil {
					// We never expect pasting to fail: break from the poll.
					return testing.PollBreak(errors.Wrap(err, "failed to paste"))
				}
				attempt++
				if got == row.wantPastedData {
					msSinceStart := time.Since(start).Seconds() * 1000
					msToPaste := time.Since(afterCopy).Seconds() * 1000
					msToCopy := msSinceStart - msToPaste
					s.Logf("Found expected paste data on attempt #%d; copy took %0.3f ms, paste worked after %0.3f ms", attempt, msToCopy, msToPaste)
					return nil
				}
				return errors.Errorf("after %d paste attempts, the pasted value was %q instead of %q", attempt, got, row.wantPastedData)
			}, &testing.PollOptions{
				// Running the actual paste operation from Chrome to Android can
				// take a long time even for a single iteration (e.g. around 1
				// second), so we are forced to give a relatively high upper bound
				// for the overall timeout.
				Timeout:  22 * time.Second,
				Interval: 3 * time.Second,
			})
			if err != nil {
				s.Fatal("Failed during paste retry loop: ", err)
			}
		})
	}

	// TODO(ruanc): Copying big text (500Kb) is blocked by https://bugs.chromium.org/p/chromium/issues/detail?id=916882
}

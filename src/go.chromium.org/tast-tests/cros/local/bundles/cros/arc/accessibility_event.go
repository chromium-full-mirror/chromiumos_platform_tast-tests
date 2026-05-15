// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/chromevox"
	"go.chromium.org/tast-tests/cros/local/arc"
	arca11y "go.chromium.org/tast-tests/cros/local/bundles/cros/arc/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type axEventTestStep struct {
	keys      string          // a sequence of keys to invoke.
	target    a11y.FindParams // expected params of the event target.
	eventType event.Event     // an expected event type from the focused node.
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     AccessibilityEvent,
		Desc:     "Checks accessibility events in Chrome are as expected with ARC enabled",
		Contacts: []string{"arc-framework+tast@google.com", "hirokisato@chromium.org", "dtseng@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Accessibility
		BugComponent: "b:165222",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithoutUIAutomator",
		Timeout:      4 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"group:hw_agnostic"},
			}},
	})
}

func runTestStep(ctx context.Context, cvconn *chromevox.Conn, tconn *chrome.TestConn, ew *input.KeyboardEventWriter, step axEventTestStep, isFirstStep bool) error {
	watcher, err := a11y.NewRootWatcher(ctx, tconn, step.eventType)
	if err != nil {
		return errors.Wrap(err, "failed to create EventWatcher")
	}
	defer watcher.Release(ctx)

	// Send a key event.
	if err := ew.Accel(ctx, step.keys); err != nil {
		return errors.Wrapf(err, "Accel(%s) returned error", step.keys)
	}

	return testing.Poll(ctx, func(ctx context.Context) error {
		events, err := watcher.WaitForEvent(ctx, 10*time.Second)
		if err != nil {
			return err
		}
		defer events.Release(ctx)

		for _, e := range events {
			if e.Target == nil {
				continue
			}
			if ok, err := e.Target.Matches(ctx, step.target); err != nil {
				return err
			} else if ok {
				return nil
			}
		}
		return errors.Errorf("expected event didn't occur. got: %+v", events)
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

func AccessibilityEvent(ctx context.Context, s *testing.State) {
	MainActivityTestSteps := []axEventTestStep{
		{
			"Tab",
			a11y.FindParams{
				Name: "OFF",
				Role: role.ToggleButton,
				Attributes: map[string]interface{}{
					"className": arca11y.ToggleButton,
					"checked":   checked.False,
				},
			},
			event.Focus,
		},
		{
			"Search+Space",
			a11y.FindParams{
				Name: "ON",
				Role: role.ToggleButton,
				Attributes: map[string]interface{}{
					"className": arca11y.ToggleButton,
					"checked":   checked.True,
				},
			},
			event.CheckedStateChanged,
		},
		{
			"Tab",
			a11y.FindParams{
				Name: "CheckBox",
				Role: role.CheckBox,
				Attributes: map[string]interface{}{
					"className": arca11y.CheckBox,
					"checked":   checked.False,
				},
			},
			event.Focus,
		},
		{
			"Search+Space",
			a11y.FindParams{
				Name: "CheckBox",
				Role: role.CheckBox,
				Attributes: map[string]interface{}{
					"className": arca11y.CheckBox,
					"checked":   checked.True,
				},
			},
			event.CheckedStateChanged,
		},
		{
			"Tab",
			a11y.FindParams{
				Name: "CheckBoxWithStateDescription",
				Role: role.CheckBox,
				Attributes: map[string]interface{}{
					"className": arca11y.CheckBox,
					"checked":   checked.False,
				},
			},
			event.Focus,
		},
		{
			"Tab",
			a11y.FindParams{
				Name: "seekBar",
				Role: role.Slider,
				Attributes: map[string]interface{}{
					"className":     arca11y.SeekBar,
					"valueForRange": 25,
				},
			},
			event.Focus,
		},
		{
			"=",
			a11y.FindParams{
				Name: "seekBar",
				Role: role.Slider,
				Attributes: map[string]interface{}{
					"className":     arca11y.SeekBar,
					"valueForRange": 26,
				},
			},
			event.RangeValueChanged,
		},
		{
			"Tab",
			a11y.FindParams{
				Role: role.Slider,
				Attributes: map[string]interface{}{
					"className":     arca11y.SeekBar,
					"valueForRange": 3,
				},
			},
			event.Focus,
		},
		{
			"-",
			a11y.FindParams{
				Role: role.Slider,
				Attributes: map[string]interface{}{
					"className":     arca11y.SeekBar,
					"valueForRange": 2,
				},
			},
			event.RangeValueChanged,
		},
	}

	EditTextActivityTestSteps := []axEventTestStep{
		{
			"Tab",
			a11y.FindParams{
				Name: "contentDescription",
				Role: role.TextField,
				Attributes: map[string]interface{}{
					"className": arca11y.EditText,
				},
			},
			event.Focus,
		},
		{
			"a",
			a11y.FindParams{
				Name: "contentDescription",
				Role: role.TextField,
				Attributes: map[string]interface{}{
					"className": arca11y.EditText,
					"value":     "a",
				},
			},
			event.ValueInTextFieldChanged,
		},
	}

	LiveRegionActivityTestSteps := []axEventTestStep{
		{
			"Tab",
			a11y.FindParams{
				Role: role.Button,
				Attributes: map[string]interface{}{
					"className": arca11y.Button,
					"name": regexp.MustCompile(
						`(CHANGE POLITE LIVE REGION|Change Polite Live Region)`,
					),
				},
			},
			event.Focus,
		},
		{
			"Enter",
			a11y.FindParams{
				Name: "Updated polite text",
				Role: role.StaticText,
				Attributes: map[string]interface{}{
					"className": arca11y.TextView,
				},
			},
			event.LiveRegionChanged,
		}, {
			"Tab",
			a11y.FindParams{
				Role: role.Button,
				Attributes: map[string]interface{}{
					"className": arca11y.Button,
					"name": regexp.MustCompile(
						`(CHANGE ASSERTIVE LIVE REGION|Change Assertive Live Region)`,
					),
				},
			},
			event.Focus,
		},
		{
			"Enter",
			a11y.FindParams{
				Name: "Updated assertive text",
				Role: role.StaticText,
				Attributes: map[string]interface{}{
					"className": arca11y.TextView,
				},
			},
			event.LiveRegionChanged,
		},
	}

	d := s.FixtValue().(*arc.PreData)
	a := d.ARC
	cr := d.Chrome

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	cvconn, cleanup, err := arca11y.SetUpChromeVox(ctx, s, cr, a, tconn)
	if err != nil {
		s.Fatal("Failed to setup ChromeVox: ", err)
	}
	defer cleanup(cleanupCtx)

	ew, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Error with creating EventWriter from keyboard: ", err)
	}
	defer ew.Close(ctx)

	for _, test := range []struct {
		activity arca11y.TestActivity
		steps    []axEventTestStep
	}{
		{arca11y.MainActivity, MainActivityTestSteps},
		{arca11y.EditTextActivity, EditTextActivityTestSteps},
		{arca11y.LiveRegionActivity, LiveRegionActivityTestSteps},
	} {
		s.Run(ctx, test.activity.Name, func(ctx context.Context, s *testing.State) {
			arca11y.AttachUIFaillog(ctx, s, tconn, "uifaillog-"+test.activity.Name)
			arca11y.AttachSystemFaillog(ctx, s, a, "dumpsys-"+test.activity.Name)

			cleanup, err = arca11y.StartActivityWithChromeVox(ctx, s, a, tconn, cvconn, test.activity)
			if err != nil {
				s.Fatal("Failed to setup: ", err)
			}
			defer cleanup(cleanupCtx)

			for i, step := range test.steps {
				if err := runTestStep(ctx, cvconn, tconn, ew, step, i == 0); err != nil {
					s.Fatalf("Failed to run a test step %+v: %v", step, err)
				}
			}
		})
	}
}

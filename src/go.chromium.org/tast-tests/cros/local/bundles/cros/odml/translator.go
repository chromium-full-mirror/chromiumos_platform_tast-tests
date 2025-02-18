// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package odml

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Translator,
		Desc: "Checks the translator is functioning correctly",
		Contacts: []string{
			"chingkang@google.com",
			"cros-odml-foundations-eng@google.com",
		},
		BugComponent: "b:1445284",
		SoftwareDeps: []string{"chrome"},
		// No attributes yet because this currently needs to be run manually to avoid DLC issues.
		HardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
		Params: []testing.Param{{
			Name: "de",
			Val:  "de",
		}, {
			Name: "fr",
			Val:  "fr",
		}, {
			Name: "ja",
			Val:  "ja",
		}},
	})
}

func Translator(ctx context.Context, s *testing.State) {
	lang := s.Param().(string)
	// It checks whether there is no error occurring within the translation between English and |lang|.
	// However, the correctness of translation is not verified.
	translation, err := translate(ctx, "Hello world!", "en", lang)
	if err != nil {
		s.Fatalf("Failed to translate from English to %s: %s", lang, err)
	}
	if _, err := translate(ctx, translation, lang, "en"); err != nil {
		s.Fatalf("Failed to translate from %s to English: %s", lang, err)
	}
}

func translate(ctx context.Context, input, source, target string) (string, error) {
	output, err := testexec.CommandContext(ctx,
		"translator_console",
		"--input="+input,
		"--source="+source,
		"--target="+target,
	).Output(testexec.DumpLogOnError)
	return string(output), err
}

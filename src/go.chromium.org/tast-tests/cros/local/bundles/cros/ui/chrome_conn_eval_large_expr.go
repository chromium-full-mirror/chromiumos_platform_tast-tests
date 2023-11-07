// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ui contains local Tast tests that exercise Chrome's integration with the rest of ChromeOS.
package ui

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeConnEvalLargeExpr,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests Chrome Conn.Eval with a very large expression",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "jonfan@google.com"},
		BugComponent: "b:1034649",
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Val: browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

func ChromeConnEvalLargeExpr(ctx context.Context, s *testing.State) {
	bt := s.Param().(browser.Type)
	cr, err := browserfixt.NewChrome(ctx, bt, lacrosfixt.NewConfig())
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}

	url := "about:blank"
	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		s.Fatalf("Failed to create new connection to %s: %s", url, err)
	}

	var result interface{}
	// Create a large statement of 100MB, minus some overhead.
	size := 1024*1024*100 - 1000
	statement := fmt.Sprintf(`(() => {
		/* %s */
		return 1 + 2
		})()`, randSeq(size))

	testing.ContextLog(ctx, "Byte length of statement: ", len([]byte(statement)))

	if err := conn.Eval(ctx, statement, &result); err != nil {
		s.Fatal("Failed when calling conn.Eval(): ", err)
	}

	testing.ContextLog(ctx, "Result ", result)

	if diff := cmp.Diff(result, 3.0); diff != "" {
		s.Fatalf("Eval mismatch (-got +want):%s", diff)
	}
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

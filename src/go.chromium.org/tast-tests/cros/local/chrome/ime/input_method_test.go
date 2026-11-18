// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ime

import (
	"testing"

	"go.chromium.org/tast-tests/cros/local/chrome"
)

func TestSupportsNativeMojoReadiness(t *testing.T) {
	cases := []struct {
		im   InputMethod
		want bool
	}{
		// Native Mojo engines that always connect to ImeService when VK is disabled.
		{im: Japanese, want: true},
		{im: JapaneseWithUSKeyboard, want: true},
		{im: Korean, want: true},
		{im: Cantonese, want: true},
		{im: ChineseArray, want: true},
		{im: ChineseCangjie, want: true},
		{im: ChineseDayi, want: true},
		{im: ChinesePinyin, want: true},
		{im: ChineseQuick, want: true},
		{im: ChineseTraditionalPinyin, want: true},
		{im: ChineseWubi, want: true},
		{im: ChineseZhuyin, want: true},
		{im: ArabicTransliteration, want: true},
		{im: GreekTransliteration, want: true},
		{im: Gujarati, want: true},
		{im: HebrewTransliteration, want: true},
		{im: Hindi, want: true},
		{im: Kannada, want: true},
		{im: Malayalam, want: true},
		{im: Marathi, want: true},
		{im: NepaliTransliteration, want: true},
		{im: Odia, want: true},
		{im: PersianTransliteration, want: true},
		{im: Punjabi, want: true},
		{im: Sanskrit, want: true},
		{im: Tamil, want: true},
		{im: Telugu, want: true},
		{im: Urdu, want: true},

		// Excluded engines: FST keyboard layouts, rule-based engines,
		// non-native transliteration engines, and zero value.
		{im: EnglishUS, want: false},
		{im: FrenchFrance, want: false},
		{im: ThaiWithKedmaneeKeyboard, want: false},
		{im: VietnameseTelex, want: false},
		{im: VietnameseVNI, want: false},
		{im: AmharicTransliteration, want: false},
		{im: SerbianTransliteration, want: false},
		{im: Tigrinya, want: false},
		{im: InputMethod{}, want: false},
	}

	for _, tc := range cases {
		if got := tc.im.supportsNativeMojoReadiness(); got != tc.want {
			t.Errorf("%q.supportsNativeMojoReadiness() = %v; want %v", tc.im, got, tc.want)
		}
	}
}

func TestIsVirtualKeyboardTarget(t *testing.T) {
	if isVirtualKeyboardTarget(nil) {
		t.Error("isVirtualKeyboardTarget(nil) = true; want false")
	}

	cases := []struct {
		name   string
		target chrome.Target
		want   bool
	}{
		{
			name: "chrome_vk_extension_inputview",
			target: chrome.Target{
				Type: "other",
				URL:  "chrome-extension://jkghodnilhceideoidjikpgommlajknk/inputview.html?id=us.compact.qwerty",
			},
			want: true,
		},
		{
			name: "chromium_vk_extension_inputview",
			target: chrome.Target{
				Type: "other",
				URL:  "chrome-extension://fgoepimhcoialccpbmpnnblemnepkkao/inputview.html#id=default.hwt",
			},
			want: true,
		},
		{
			name: "chrome_keyboard_webui_fallback",
			target: chrome.Target{
				Type: "other",
				URL:  "chrome://keyboard/",
			},
			want: true,
		},
		{
			name: "ime_extension_background_worker",
			target: chrome.Target{
				Type: "service_worker",
				URL:  "chrome-extension://jkghodnilhceideoidjikpgommlajknk/background.js",
			},
			want: false,
		},
		{
			name: "regular_browser_tab",
			target: chrome.Target{
				Type: "page",
				URL:  "chrome-extension://jkghodnilhceideoidjikpgommlajknk/inputview.html",
			},
			want: false,
		},
		{
			name: "non_extension_inputview_url",
			target: chrome.Target{
				Type: "other",
				URL:  "https://example.com/inputview.html",
			},
			want: false,
		},
	}

	for _, tc := range cases {
		if got := isVirtualKeyboardTarget(&tc.target); got != tc.want {
			t.Errorf("%s: isVirtualKeyboardTarget(%+v) = %v; want %v", tc.name, tc.target, got, tc.want)
		}
	}
}

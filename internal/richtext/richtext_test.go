// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package richtext

import (
	"strings"
	"testing"
)

const file = "01a186de-611d-7ae4-bda3-1f921281ef39"

// What the editor writes stays; what could run, load from elsewhere or style is gone.
func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{
		`<p><strong>Fett</strong> und <em>kursiv</em></p>`:         `<p><strong>Fett</strong> und <em>kursiv</em></p>`,
		`<p onclick="x()">a</p><script>alert(1)</script>`:          `<p>a</p>`,
		`<img src="/api/v1/files/` + file + `" alt="Bild">`:        `<img src="/api/v1/files/` + file + `" alt="Bild">`,
		`<img src="https://evil.example/x.png">`:                   ``,
		`<img src="/api/v1/files/../../etc/passwd">`:               ``,
		`<a href="javascript:alert(1)">x</a>`:                      `x`,
		`<a href="https://group24.de">g</a>`:                       `<a href="https://group24.de" rel="nofollow noreferrer noopener" target="_blank">g</a>`,
		`<p style="color:red" class="x">a</p>`:                     `<p>a</p>`,
		`<iframe src="https://x"></iframe><ul><li>eins</li></ul>`:  `<ul><li>eins</li></ul>`,
		`<table><tr><td colspan="2" style="x">a</td></tr></table>`: `<table><tr><td colspan="2">a</td></tr></table>`,
	} {
		if got := Sanitize(in); got != want {
			t.Errorf("%s\n got  %s\n want %s", in, got, want)
		}
	}
}

func TestImages(t *testing.T) {
	h := `<p>a<img src="/api/v1/files/` + file + `"></p><img src="/api/v1/files/` + file + `"><img src="/x">`
	if got := Images(h); len(got) != 1 || got[0].String() != file {
		t.Fatalf("images: %v", got)
	}
}

func TestText(t *testing.T) {
	h := `<p>Hallo <strong>Anna</strong>,</p><ul><li>eins</li><li>zwei</li></ul><p>siehe <a href="https://x.de">hier</a><img src="/api/v1/files/` + file + `"></p>`
	want := "Hallo Anna,\n\n- eins\n- zwei\nsiehe hier (https://x.de)[Bild]"
	if got := Text(h); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if got := Text("nur Text &amp; mehr"); got != "nur Text & mehr" {
		t.Fatalf("plain: %q", got)
	}
	if strings.Contains(Text(`<p>a</p><p>b</p><p></p><p></p><p>c</p>`), "\n\n\n") {
		t.Fatal("too many breaks")
	}
}

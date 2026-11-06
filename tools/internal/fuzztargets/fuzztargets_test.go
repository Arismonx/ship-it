// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package fuzztargets

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	out := "FuzzParseStatus\nok  \tgithub.com/x/gitexec\t0.2s\n" +
		"?   \tgithub.com/x/none\t[no test files]\n" +
		"FuzzA\nFuzzB\nok  \tgithub.com/x/convention\t0.1s\n" +
		"ok  \tgithub.com/x/nofuzz\t0.1s\n"
	want := []Target{
		{"github.com/x/gitexec", "FuzzParseStatus"},
		{"github.com/x/convention", "FuzzA"},
		{"github.com/x/convention", "FuzzB"},
	}
	if got := Parse([]byte(out)); !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse = %+v, want %+v", got, want)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import "testing"

func TestWantsSignoff(t *testing.T) {
	signed := "✨ feat: x\n\nSigned-off-by: Ada <ada@example.invalid>"
	tests := []struct {
		name     string
		messages []string
		guides   []string
		dcoApp   bool
		want     bool
	}{
		{"empty repository", nil, nil, false, false},
		{"most commits signed off", []string{signed, signed, "🐛 fix: y"}, nil, false, true},
		{"half is not most", []string{signed, "🐛 fix: y"}, nil, false, false},
		{"guide names the DCO", nil, []string{"We use the DCO for every commit."}, false, true},
		{"guide names the trailer", nil, []string{"Add a signed-off-by line."}, false, true},
		{"a word containing dco is not the DCO", nil, []string{"Run the dcoverage tool."}, false, false},
		{"DCO app", nil, nil, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WantsSignoff(tt.messages, tt.guides, tt.dcoApp); got != tt.want {
				t.Errorf("WantsSignoff = %v, want %v", got, tt.want)
			}
		})
	}
}

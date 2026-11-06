// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package retention

import (
	"reflect"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func at(id string, daysAgo int, hour int) Item {
	day := now.AddDate(0, 0, -daysAgo)
	return Item{ID: id, CreatedAt: time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, time.UTC)}
}

func TestExpired(t *testing.T) {
	items := []Item{ // newest first
		at("today-2", 0, 11), at("today-1", 0, 9),
		at("d1-late", 1, 18), at("d1-early", 1, 8),
		at("d40", 40, 10),
		at("d50-protected", 50, 10),
	}
	policy := Policy{KeepLatest: 1, DailyWindow: 30 * 24 * time.Hour}
	got := Expired(items, now, policy, map[string]bool{"d50-protected": true})
	// today-2 is the latest; today-1 shares its day; d1-late is that day's
	// newest; d40 is outside the window.
	want := []string{"today-1", "d1-early", "d40"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Expired = %v, want %v", got, want)
	}
}

func TestDefaultPolicyKeepsRecentWork(t *testing.T) {
	var items []Item
	for i := range 20 {
		items = append(items, at("old", 100, i))
	}
	if got := Expired(items, now, DefaultPolicy, nil); len(got) != 0 {
		t.Fatalf("the 20 latest checkpoints must survive whatever their age, expired %d", len(got))
	}
}

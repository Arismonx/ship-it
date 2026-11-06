// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import "strings"

// MinHabitSample is how many conventional subjects the history must hold
// before ship-it follows its habits rather than its own defaults.
const MinHabitSample = 5

// DetectGitmojiPlacement reads how a repository writes its gitmoji, from
// recent commit subjects: before the type ("✨ feat: x"), after the colon
// ("feat: ✨ x") or not at all ("feat: x"). Subjects that are not
// conventional headers are ignored. It reports false when fewer than
// MinHabitSample subjects tell: a young repository gets ship-it's default.
// The most frequent placement wins; a tie goes to ship-it's default order
// (first, after the type, none). It reads types with the built-in table
// (Rules.DetectGitmojiPlacement).
func DetectGitmojiPlacement(subjects []string) (GitmojiPlacement, bool) {
	return Rules{}.DetectGitmojiPlacement(subjects)
}

// DetectGitmojiPlacement is DetectGitmojiPlacement with the repository's
// commit types (ship.commits.types): a subject of a type it added counts, one
// of a type it removed does not.
func (r Rules) DetectGitmojiPlacement(subjects []string) (GitmojiPlacement, bool) {
	var counts [3]int
	total := 0
	for _, s := range subjects {
		s = strings.TrimSpace(s)
		if _, err := r.ParseHeader(s); err != nil {
			continue
		}
		total++
		switch {
		case hasLeadingGitmoji(s):
			counts[GitmojiFirst]++
		case hasGitmojiAfterColon(s):
			counts[GitmojiAfterType]++
		default:
			counts[GitmojiOmitted]++
		}
	}
	if total < MinHabitSample {
		return GitmojiFirst, false
	}
	best := GitmojiFirst
	for _, p := range []GitmojiPlacement{GitmojiAfterType, GitmojiOmitted} {
		if counts[p] > counts[best] {
			best = p
		}
	}
	return best, true
}

func hasLeadingGitmoji(s string) bool {
	_, _, ok := splitLeadingGitmoji(s)
	return ok
}

func hasGitmojiAfterColon(s string) bool {
	_, subject, found := strings.Cut(s, ": ")
	if !found {
		return false
	}
	_, _, ok := splitLeadingGitmoji(subject)
	return ok
}

// ParsePlacement reads the ship.commits.gitmoji setting; "auto" and unknown
// values report false, for the caller to detect the habit.
func ParsePlacement(setting string) (GitmojiPlacement, bool) {
	switch setting {
	case "first":
		return GitmojiFirst, true
	case "after-type":
		return GitmojiAfterType, true
	case "none":
		return GitmojiOmitted, true
	}
	return GitmojiFirst, false
}

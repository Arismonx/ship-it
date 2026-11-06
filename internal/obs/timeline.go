// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// Event is one journal record, decoded.
type Event struct {
	Time       time.Time      `json:"time"`
	Level      string         `json:"level"`
	Msg        string         `json:"msg"`
	Step       string         `json:"step,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Attrs      map[string]any `json:"attrs,omitempty"`
}

// Start is when the event's step began: its time minus its duration (a
// step is logged when it ends).
func (e Event) Start() time.Time { return e.Time.Add(-time.Duration(e.DurationMS) * time.Millisecond) }

// commonKeys are not repeated in Event.Attrs.
var commonKeys = map[string]bool{
	"time": true, "level": true, "msg": true, KeyStep: true, KeyDurationMS: true,
	KeySeverityNumber: true, KeyRunID: true, KeyRepoHash: true,
}

// maxLine bounds one journal line when reading it back.
const maxLine = 1 << 20

// ReadEvents decodes a JSONL journal. Lines that are not JSON objects are
// skipped: a truncated last line must not hide the rest.
func ReadEvents(r io.Reader) ([]Event, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), maxLine)
	var events []Event
	for scanner.Scan() {
		var raw map[string]any
		if json.Unmarshal(scanner.Bytes(), &raw) != nil {
			continue
		}
		e := Event{Attrs: map[string]any{}}
		if s, ok := raw["time"].(string); ok {
			e.Time, _ = time.Parse(time.RFC3339Nano, s)
		}
		e.Level, _ = raw["level"].(string)
		e.Msg, _ = raw["msg"].(string)
		e.Step, _ = raw[KeyStep].(string)
		if d, ok := raw[KeyDurationMS].(float64); ok {
			e.DurationMS = int64(d)
		}
		for k, v := range raw {
			if !commonKeys[k] {
				e.Attrs[k] = v
			}
		}
		events = append(events, e)
	}
	return events, scanner.Err()
}

// Steps keeps the events that describe a step (they carry "step"), sorted by
// start time.
func Steps(events []Event) []Event {
	var steps []Event
	for _, e := range events {
		if e.Step != "" {
			steps = append(steps, e)
		}
	}
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].Start().Before(steps[j].Start()) })
	return steps
}

// detailKeys are shown after the step name, in this order.
var detailKeys = []string{"cmd", "method", "path", "status", "exit_code", "gate", KeyOperationID, "rate_remaining", "error"}

// WriteTimeline writes steps as an aligned timeline: offset from the first
// step, duration, step name and details. Failed steps are marked with ✗.
func WriteTimeline(w io.Writer, steps []Event) error {
	if len(steps) == 0 {
		return nil
	}
	origin := steps[0].Start()
	width := 0
	for _, s := range steps {
		width = max(width, len(s.Step))
	}
	for _, s := range steps {
		mark := " "
		if s.Level == "WARN" || s.Level == "ERROR" || strings.HasSuffix(s.Msg, ".failed") {
			mark = "✗"
		}
		var details []string
		for _, k := range detailKeys {
			if v, ok := s.Attrs[k]; ok && fmt.Sprint(v) != "" {
				details = append(details, k+"="+compact(v))
			}
		}
		offset := s.Start().Sub(origin).Seconds()
		if _, err := fmt.Fprintf(w, "%s +%8.3fs %7dms  %-*s  %s\n", mark, offset, s.DurationMS, width, s.Step, strings.Join(details, " ")); err != nil {
			return err
		}
	}
	return nil
}

func compact(v any) string {
	var s string
	switch x := v.(type) {
	case []any:
		parts := make([]string, len(x))
		for i, p := range x {
			parts[i] = fmt.Sprint(p)
		}
		s = strings.Join(parts, " ")
	case float64:
		s = fmt.Sprint(int64(x))
	default:
		s = fmt.Sprint(v)
	}
	if r := []rune(s); len(r) > 80 {
		s = string(r[:79]) + "…"
	}
	return s
}

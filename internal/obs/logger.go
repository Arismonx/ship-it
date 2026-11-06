// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Attribute keys shared by journal records (docs/reference/logs.md §3).
const (
	KeySeverityNumber = "severity_number"
	KeyRunID          = "run_id"
	KeyOperationID    = "op_id"
	KeyTraceID        = "trace_id"
	KeySpanID         = "span_id"
	KeyParentSpanID   = "parent_span_id"
	KeyStep           = "step"
	KeyDurationMS     = "duration_ms"
	KeyRepoHash       = "repo_hash"
)

// pendingLimit bounds what is buffered before the journal file is opened.
const pendingLimit = 256 << 10

// Journal is the technical journal of one ship-it run: INFO and above go to
// a JSONL file, every record goes to the black box, and --debug echoes
// records on stderr. Every value is scrubbed of private data first.
type Journal struct {
	Logger   *slog.Logger
	BlackBox *BlackBox
	RunID    RunID

	file    *lateWriter
	console *console
	repo    atomic.Pointer[string]
	started time.Time
	// unexpected is set by MarkUnexpected: the run failed in a way ship-it
	// has no answer for, which is what the black box is kept for.
	unexpected atomic.Bool

	mu        sync.Mutex
	dir       string
	handle    *os.File
	retention Retention
}

// JournalOptions configure NewJournal.
type JournalOptions struct {
	// File receives INFO and above as JSONL. When nil, records are buffered
	// until Attach opens <dir>/<run_id>.jsonl, or dropped after Discard.
	File io.Writer
	// FileLevel is the minimum level written to File (default Info).
	FileLevel slog.Leveler
	// BlackBoxCapacity bounds the black box (default DefaultBlackBoxCapacity).
	BlackBoxCapacity int
	// RunID identifies the run; a new one is generated when empty.
	RunID RunID
	// Scrubber removes private data; nil still removes e-mails, tokens, URL
	// credentials and common absolute paths.
	Scrubber *Scrubber
}

// NewJournal builds the run journal.
func NewJournal(opts JournalOptions) *Journal {
	runID := opts.RunID
	if runID == "" {
		runID = NewRunID()
	}
	j := &Journal{
		BlackBox:  NewBlackBox(opts.BlackBoxCapacity),
		RunID:     runID,
		file:      &lateWriter{},
		console:   &console{},
		started:   time.Now(),
		retention: DefaultRetention,
	}
	if opts.File != nil {
		j.file.attach(opts.File)
	}
	level := opts.FileLevel
	if level == nil {
		level = slog.LevelInfo
	}
	fan := fanOut{
		newJSONHandler(j.BlackBox, slog.LevelDebug),
		newJSONHandler(j.file, level),
		consoleHandler{j.console, slog.NewTextHandler(j.console, &slog.HandlerOptions{
			Level: slog.LevelDebug,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if len(groups) == 0 && (a.Key == slog.TimeKey || a.Key == KeyRunID || a.Key == KeyRepoHash) {
					return slog.Attr{} // noise on a terminal
				}
				return a
			},
		})},
	}
	scrubber := opts.Scrubber
	if scrubber == nil {
		scrubber = NewScrubber(nil)
	}
	root := rootHandler{next: fan, scrub: scrubber, repo: &j.repo}
	j.Logger = slog.New(root).With(slog.String(KeyRunID, string(runID)))
	return j
}

// SetRepository tags every following record with repo_hash.
func (j *Journal) SetRepository(hash string) { j.repo.Store(&hash) }

// EchoTo also writes records at level and above to w in a human-readable
// form (ship --debug writes DEBUG to stderr). A nil w stops the echo.
func (j *Journal) EchoTo(w io.Writer, level slog.Level) { j.console.set(w, level) }

// DumpBlackBox writes the black box, for example to
// "<logs>/<run_id>.blackbox.jsonl" when a run fails.
func (j *Journal) DumpBlackBox(w io.Writer) error {
	_, err := j.BlackBox.WriteTo(w)
	return err
}

func newJSONHandler(w io.Writer, level slog.Leveler) slog.Handler {
	return severityHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})}
}

// OTelSeverity maps a slog level to an OpenTelemetry SeverityNumber:
// slog Debug(-4), Info(0), Warn(4), Error(8) become 5, 9, 13 and 17,
// the first value of the DEBUG, INFO, WARN and ERROR ranges.
func OTelSeverity(level slog.Level) int {
	n := 9 + int(level)
	switch {
	case n < 1:
		return 1
	case n > 24:
		return 24
	}
	return n
}

// rootHandler scrubs every value and adds the run-wide attributes: the trace
// context carried by the context, and the repository hash.
type rootHandler struct {
	next  slog.Handler
	scrub *Scrubber
	repo  *atomic.Pointer[string]
}

func (h rootHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h rootHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, h.scrub.String(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.scrub.Attr(a))
		return true
	})
	if tc, ok := TraceFrom(ctx); ok {
		out.AddAttrs(slog.String(KeyTraceID, tc.TraceID), slog.String(KeySpanID, tc.SpanID))
		if tc.ParentSpanID != "" {
			out.AddAttrs(slog.String(KeyParentSpanID, tc.ParentSpanID))
		}
	}
	if repo := h.repo.Load(); repo != nil && *repo != "" {
		out.AddAttrs(slog.String(KeyRepoHash, *repo))
	}
	return h.next.Handle(ctx, out)
}

func (h rootHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	scrubbed := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		scrubbed[i] = h.scrub.Attr(a)
	}
	return rootHandler{next: h.next.WithAttrs(scrubbed), scrub: h.scrub, repo: h.repo}
}

func (h rootHandler) WithGroup(name string) slog.Handler {
	return rootHandler{next: h.next.WithGroup(name), scrub: h.scrub, repo: h.repo}
}

// severityHandler adds the OpenTelemetry severity number to every record.
type severityHandler struct{ slog.Handler }

func (h severityHandler) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(slog.Int(KeySeverityNumber, OTelSeverity(r.Level)))
	return h.Handler.Handle(ctx, r)
}

func (h severityHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return severityHandler{h.Handler.WithAttrs(attrs)}
}

func (h severityHandler) WithGroup(name string) slog.Handler {
	return severityHandler{h.Handler.WithGroup(name)}
}

// fanOut sends each record to every handler that is enabled for its level.
type fanOut []slog.Handler

func (f fanOut) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f fanOut) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			errs = append(errs, h.Handle(ctx, r.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (f fanOut) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(fanOut, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (f fanOut) WithGroup(name string) slog.Handler {
	out := make(fanOut, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(name)
	}
	return out
}

// lateWriter buffers what is written until a destination is attached, so
// that records logged before the log directory is known are not lost.
type lateWriter struct {
	mu      sync.Mutex
	w       io.Writer
	discard bool
	pending bytes.Buffer
}

func (l *lateWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.w != nil:
		return l.w.Write(p)
	case l.discard:
	case l.pending.Len()+len(p) <= pendingLimit:
		l.pending.Write(p)
	}
	return len(p), nil
}

func (l *lateWriter) attach(w io.Writer) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.w, l.discard = w, false
	_, err := l.pending.WriteTo(w)
	return err
}

func (l *lateWriter) drop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.w, l.discard = nil, true
	l.pending.Reset()
}

// console is the --debug echo; it is off until set.
type console struct {
	mu    sync.Mutex
	w     io.Writer
	level slog.Level
}

func (c *console) set(w io.Writer, level slog.Level) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.w, c.level = w, level
}

func (c *console) enabled(level slog.Level) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.w != nil && level >= c.level
}

func (c *console) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.w == nil {
		return len(p), nil
	}
	return c.w.Write(p)
}

type consoleHandler struct {
	c *console
	slog.Handler
}

func (h consoleHandler) Enabled(_ context.Context, level slog.Level) bool { return h.c.enabled(level) }

func (h consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return consoleHandler{h.c, h.Handler.WithAttrs(attrs)}
}

func (h consoleHandler) WithGroup(name string) slog.Handler {
	return consoleHandler{h.c, h.Handler.WithGroup(name)}
}

// MarkUnexpected records that the run hit an error ship-it has no fault for;
// the surface then keeps the black box.
func (j *Journal) MarkUnexpected() { j.unexpected.Store(true) }

// Unexpected reports whether MarkUnexpected was called.
func (j *Journal) Unexpected() bool { return j.unexpected.Load() }

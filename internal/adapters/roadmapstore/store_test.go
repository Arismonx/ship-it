// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package roadmapstore

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/roadmap"
)

type dir string

func (d dir) GitDir(context.Context) (string, error) { return string(d), nil }

func TestLoadAndSave(t *testing.T) {
	d := t.TempDir()
	s := New(dir(d))
	ctx := context.Background()
	if m, err := s.Load(ctx); err != nil || len(m) != 0 {
		t.Fatalf("first load = %v, %v", m, err)
	}
	want := roadmap.Memory{12: true, 13: false}
	if err := s.Save(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %v, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(d, "ship", "roadmap.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load(ctx); err != nil || len(got) != 0 {
		t.Fatalf("a broken file is a first sync: %v, %v", got, err)
	}
}

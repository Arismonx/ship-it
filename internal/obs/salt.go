// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// SaltSize is the size of the installation salt, in bytes.
const SaltSize = 32

// Waiting for another run that is writing the salt: at most saltAttempts
// reads, saltWait apart. A file still incomplete after staleSalt was left by
// a run that died halfway and is replaced.
const (
	saltAttempts = 100
	saltWait     = 10 * time.Millisecond
	staleSalt    = 5 * time.Second
)

// ErrBadSalt is returned when the salt file cannot be read back whole.
var ErrBadSalt = errors.New("obs: the installation salt file is unreadable; delete it to create a new one")

// InstallSalt returns the random salt of this installation, stored at path
// (platform.Environment.SaltFile), creating it on first use: 32 bytes from
// crypto/rand, mode 0600. The file is created with O_EXCL, so concurrent
// runs agree on one salt: a run that loses the race reads the winner's.
func InstallSalt(path string) ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return nil, err
	}
	for range saltAttempts {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
		if err == nil {
			salt := make([]byte, SaltSize)
			_, _ = rand.Read(salt) // crypto/rand.Read never fails since Go 1.24
			_, writeErr := f.Write(salt)
			if err := errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
				return nil, err
			}
			return salt, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err == nil && len(data) == SaltSize {
			return data, nil
		}
		// Another run is writing it, or one died halfway.
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > staleSalt {
			_ = os.Remove(path)
			continue
		}
		time.Sleep(saltWait)
	}
	return nil, ErrBadSalt
}

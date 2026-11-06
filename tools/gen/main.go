// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Command gen writes the generated files of the repository. Run it from the
// module root:
//
//	go run ./tools/gen
//
// Its test fails when a generated file is out of date.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GregoireF/ship-it/internal/core/config"
)

// generated maps each output path, relative to the module root, to its
// content; root is the module root, for the files made from other files.
func generated(root string) (map[string][]byte, error) {
	files := map[string][]byte{
		filepath.Join("schemas", "ship.schema.json"):                          config.Schema(),
		filepath.Join("schemas", "policy.schema.json"):                        config.PolicySchema(),
		filepath.Join("schemas", "user.schema.json"):                          config.UserSchema(),
		filepath.Join("docs", "reference", "settings.md"):                     config.Reference(),
		filepath.Join("extensions", "vscode", "src", "generated", "tools.ts"): typeScriptTools(),
	}
	marketplace, err := marketplaceFiles(root)
	if err != nil {
		return nil, err
	}
	for path, content := range marketplace {
		files[path] = content
	}
	notices, err := thirdPartyNotices(root)
	if err != nil {
		return nil, err
	}
	files[noticesFile] = notices
	errors, err := errorsReference(root)
	if err != nil {
		return nil, err
	}
	files[filepath.FromSlash(errorsFile)] = errors
	return files, nil
}

func main() {
	files, err := generated(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "gen:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "gen:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", filepath.ToSlash(path))
	}
}

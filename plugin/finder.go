/*
Copyright 2017 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package plugin

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type finder struct {
	validPrefixes    []string
	searchPaths      []string
	coreRootCommands []string
}

func (o *finder) find() map[string]string {
	plugins := make(map[string]string)

	for _, dir := range unique(o.searchPaths) {
		if len(strings.TrimSpace(dir)) == 0 {
			continue
		}

		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() {
				continue
			}
			if !hasValidPrefix(f.Name(), o.validPrefixes) {
				continue
			}

			// Prevent overriding core commands or adding any subcommands under core commands.
			if isUnderCoreRootCommands(f.Name(), o.coreRootCommands) {
				continue
			}

			path := filepath.Join(dir, f.Name())
			if !isExecutable(path) {
				continue
			}

			nameParts := strings.Split(f.Name(), "-")
			if len(nameParts) > 1 {
				// the first argument is always one of `validPrefixes` (i.e: "trellis") for a plugin binary
				nameParts = nameParts[1:]
			}
			name := strings.Join(nameParts, " ")

			// TODO: Handle overshadowed plugins, i.e: plugins with the same binary name.
			plugins[name] = path
		}
	}

	return plugins
}

func isExecutable(fullPath string) bool {
	info, err := os.Stat(fullPath)
	if err != nil {
		return false
	}

	if runtime.GOOS == "windows" {
		fileExt := strings.ToLower(filepath.Ext(fullPath))

		switch fileExt {
		case ".bat", ".cmd", ".com", ".exe", ".ps1":
			return true
		}
		return false
	}

	if m := info.Mode(); !m.IsDir() && m&0111 != 0 {
		return true
	}

	return false
}

func hasValidPrefix(filepath string, validPrefixes []string) bool {
	for _, prefix := range validPrefixes {
		if !strings.HasPrefix(filepath, prefix+"-") {
			continue
		}
		return true
	}
	return false
}

func isUnderCoreRootCommands(filepath string, coreRootCommands []string) bool {
	// the first argument is always one of `validPrefixes` (i.e: "trellis") for a plugin binary
	rootCommand := strings.Split(filepath, "-")[1]

	for _, v := range coreRootCommands {
		if v == rootCommand {
			return true
		}
	}
	return false
}

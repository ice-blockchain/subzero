// SPDX-License-Identifier: ice License 1.0

//go:build test

package cfg

import (
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/cockroachdb/errors"
)

func init() {
	mustInit(findAllApplicationConfigFiles()...)
}

func findAllApplicationConfigFiles() []string {
	var files []string
	var hints []string

	if p, err := os.Getwd(); err == nil {
		hints = append(hints, p)
	}
	if p, err := os.Executable(); err == nil {
		hints = append(hints, path.Dir(filepath.Join(p, "..")))
	}

	for _, dir := range hints {
		pattern := filepath.Join(dir, ".testdata", "application.yaml")
		if f, err := filepath.Glob(pattern); err != nil {
			log.Println(errors.Wrapf(err, "glob failed for [%v]", pattern))
		} else {
			files = append(files, f...)
		}
		pattern = filepath.Join(dir, "application.yaml")
		if f, err := filepath.Glob(pattern); err != nil {
			log.Println(errors.Wrapf(err, "glob failed for [%v]", pattern))
		} else {
			files = append(files, f...)
		}
	}
	files = append(files, relativeFiles()...)

	return files

}

func relativeFiles() []string {
	var files []string
	//nolint:dogsled // Because those 3 blank identifiers are useless
	_, callerFile, _, _ := runtime.Caller(0)
	pattern := filepath.Join(filepath.Dir(callerFile), "..", "application.yaml")
	if f, err := filepath.Glob(pattern); err != nil {
		log.Println(errors.Wrapf(err, "glob failed for [%v]", pattern))
	} else {
		files = append(files, f...)
	}
	pattern = filepath.Join(filepath.Dir(callerFile), "..", "..", "application.yaml")
	if f, err := filepath.Glob(pattern); err != nil {
		log.Println(errors.Wrapf(err, "glob failed for [%v]", pattern))
	} else {
		files = append(files, f...)
	}

	return files
}

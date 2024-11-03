// SPDX-License-Identifier: ice License 1.0

//go:build !test

package dvm

func PublicKey() (string, error) {
	panic("`test` tag must be used to run DVM tests")
}

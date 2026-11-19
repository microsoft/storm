// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.

package libvirt

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// nameSep separates the namespace, the run ID and the rest of an object name.
// It is outside the alphabet allowed in all three, so an ownership check by
// prefix is unambiguous: "ci_ab12_" can never match an object of namespace
// "ci-2" or of run "ab123".
const nameSep = "_"

var (
	validNameRE  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	validRunIDRE = regexp.MustCompile(`^[a-z0-9]{1,16}$`)
)

// maxNameLen bounds namespace and VM names so that the composed libvirt names
// stay well within libvirt and kernel limits.
const maxNameLen = 32

func validateName(kind, s string) error {
	if s == "" {
		return fmt.Errorf("%s must not be empty", kind)
	}
	if len(s) > maxNameLen {
		return fmt.Errorf("%s %q is longer than %d characters", kind, s, maxNameLen)
	}
	if !validNameRE.MatchString(s) {
		return fmt.Errorf("%s %q is invalid: use lowercase letters, digits and single hyphens, starting and ending with a letter or digit", kind, s)
	}
	return nil
}

func validateRunID(s string) error {
	if !validRunIDRE.MatchString(s) {
		return fmt.Errorf("run ID %q is invalid: use 1-16 lowercase letters or digits", s)
	}
	return nil
}

func newRunID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate run ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// scope identifies the objects of one run: every libvirt object it creates is
// named "<namespace>_<run>_<something>".
type scope struct {
	ns  string
	run string
}

func (s scope) prefix() string { return s.ns + nameSep + s.run + nameSep }

func (s scope) String() string { return s.ns + nameSep + s.run }

func (s scope) networkName() string { return s.prefix() + "net" }

func (s scope) poolName() string { return s.prefix() + "pool" }

func (s scope) poolPath() string { return "/var/lib/libvirt/" + s.poolName() }

func (s scope) domainName(vm string) string { return s.prefix() + vm }

func (s scope) volumeName(vm string, index int, format string) string {
	ext := "img"
	if format == formatQcow2 {
		ext = "qcow2"
	}
	return fmt.Sprintf("%s%s-disk%d.%s", s.prefix(), vm, index, ext)
}

// owns reports whether a libvirt object name belongs to this run.
func (s scope) owns(name string) bool {
	return strings.HasPrefix(name, s.prefix())
}

// runsIn returns the distinct run IDs of namespace ns found among names, in
// first-seen order.
func runsIn(ns string, names []string) []string {
	var runs []string
	seen := map[string]bool{}
	for _, n := range names {
		rest, ok := strings.CutPrefix(n, ns+nameSep)
		if !ok {
			continue
		}
		run, _, ok := strings.Cut(rest, nameSep)
		if !ok || !validRunIDRE.MatchString(run) || seen[run] {
			continue
		}
		seen[run] = true
		runs = append(runs, run)
	}
	return runs
}

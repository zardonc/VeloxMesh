package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var profileKinds = []string{"cpuprofile", "memprofile", "blockprofile", "mutexprofile", "trace"}

func profileFlags(test string) string {
	if os.Getenv("SHIP_PROFILE") != "true" {
		return ""
	}
	flags := " -test.blockprofilerate=1 -test.mutexprofilefraction=1"
	for _, kind := range profileKinds {
		flags += " -test." + kind + "=/tmp/veloxmesh-" + test + "." + kind
	}
	return flags
}

func (r *runner) fetchProfiles(test string) error {
	if os.Getenv("SHIP_PROFILE") != "true" {
		return nil
	}
	var failures []error
	for _, kind := range profileKinds {
		output, err := r.command("cat /tmp/veloxmesh-" + test + "." + kind)
		if err != nil {
			failures = append(failures, fmt.Errorf("fetch %s: %w", kind, err))
			continue
		}
		if err := os.WriteFile(filepath.Join(artifacts, test+"."+kind), output, 0600); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

//go:build phase29preflight

package app

import "testing"

func TestLiveControlledExactMiss(t *testing.T) {
	controlledLoadProfile(t, controlledProfile{mode: "on", capacity: "0", reuseMode: "exact"})
}

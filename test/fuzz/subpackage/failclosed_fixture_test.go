//go:build release_fuzz_discovery_failure

package subpackage

import "testing"

func TestReleaseFuzzDiscoveryFailure(t *testing.T) {
	_ = releaseFuzzDiscoveryFailureUndefined
}

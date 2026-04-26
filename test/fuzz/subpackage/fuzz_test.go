package subpackage

import "testing"

func FuzzReleaseGateSubpackageDiscovery(f *testing.F) {
	f.Add("seed")
	f.Fuzz(func(t *testing.T, value string) {})
}

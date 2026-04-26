// Package integrity provides checksum and payload hash computation for snapshots.
package integrity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/agentsmith-project/jvs/pkg/jsonutil"
	"github.com/agentsmith-project/jvs/pkg/model"
)

// ComputeDescriptorChecksum computes SHA-256 checksum of the descriptor.
// Excludes: descriptor_checksum, integrity_state (per spec 04)
// Includes all other fields to ensure tamper detection.
func ComputeDescriptorChecksum(desc *model.Descriptor) (model.HashValue, error) {
	checksumDesc := *desc
	checksumDesc.DescriptorChecksum = ""
	checksumDesc.IntegrityState = ""

	data, err := jsonutil.CanonicalMarshal(&checksumDesc)
	if err != nil {
		return "", fmt.Errorf("canonical marshal descriptor: %w", err)
	}

	hash := sha256.Sum256(data)
	return model.HashValue(hex.EncodeToString(hash[:])), nil
}

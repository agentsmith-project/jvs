package model

// EngineType identifies the snapshot engine type used for creating snapshots.
type EngineType string

const (
	EngineJuiceFSClone EngineType = "juicefs-clone"
	EngineReflinkCopy  EngineType = "reflink-copy"
	EngineCopy         EngineType = "copy"
)

// MetadataPreservation describes metadata semantics for an engine choice.
type MetadataPreservation struct {
	Symlinks   string `json:"symlinks"`
	Hardlinks  string `json:"hardlinks"`
	Mode       string `json:"mode"`
	Timestamps string `json:"timestamps"`
	Xattrs     string `json:"xattrs"`
	ACLs       string `json:"acls"`
}

// IntegrityState represents the verification status of a snapshot.
type IntegrityState string

const (
	IntegrityVerified IntegrityState = "verified"
	IntegrityTampered IntegrityState = "tampered"
	IntegrityUnknown  IntegrityState = "unknown"
)

// HashValue is a SHA-256 hash stored as a hex string.
type HashValue string

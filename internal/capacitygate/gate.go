// Package capacitygate provides preflight free-space checks for operations that
// materialize save point payloads before mutating public state.
package capacitygate

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/agentsmith-project/jvs/pkg/errclass"
)

const (
	ErrCode                  = "E_NOT_ENOUGH_SPACE"
	DefaultSafetyMarginBytes = 16 << 20
)

type Meter interface {
	AvailableBytes(path string) (int64, error)
}

type DeviceMeter interface {
	Meter
	DeviceID(path string) (string, error)
}

type Gate struct {
	Meter             Meter
	SafetyMarginBytes int64
}

type Component struct {
	Name  string
	Path  string
	Bytes int64
}

type Request struct {
	Operation       string
	Folder          string
	Workspace       string
	SourceSavePoint string
	Path            string
	Components      []Component
	FailureMessages []string
}

type Decision struct {
	RequiredBytes     int64
	SafetyMarginBytes int64
	AvailableBytes    int64
	ShortfallBytes    int64
	ProbePath         string
}

type InsufficientError struct {
	Request  Request
	Decision Decision
}

func Default() Gate {
	return Gate{
		Meter:             StatfsMeter{},
		SafetyMarginBytes: DefaultSafetyMarginBytes,
	}
}

func (g Gate) Check(req Request) (*Decision, error) {
	meter := g.Meter
	if meter == nil {
		meter = StatfsMeter{}
	}
	margin := g.SafetyMarginBytes
	if margin < 0 {
		margin = 0
	}
	groups, order, required, err := groupComponentsByDevice(req, meter)
	if err != nil {
		return nil, err
	}
	if len(order) == 0 {
		probePath := probePath(req)
		available, err := meter.AvailableBytes(probePath)
		if err != nil {
			return nil, fmt.Errorf("check free space: %w", err)
		}
		return &Decision{
			RequiredBytes:     0,
			SafetyMarginBytes: 0,
			AvailableBytes:    available,
			ProbePath:         probePath,
		}, nil
	}

	decision := &Decision{
		RequiredBytes: required,
		ProbePath:     groups[order[0]].probePath,
	}
	for _, key := range order {
		group := groups[key]
		available, err := meter.AvailableBytes(group.probePath)
		if err != nil {
			return nil, fmt.Errorf("check free space: %w", err)
		}
		needed := saturatingAdd(group.requiredBytes, margin)
		decision.SafetyMarginBytes = saturatingAdd(decision.SafetyMarginBytes, margin)
		decision.AvailableBytes = saturatingAdd(decision.AvailableBytes, available)
		if available < needed {
			decision.ShortfallBytes = saturatingAdd(decision.ShortfallBytes, needed-available)
		}
	}
	if decision.ShortfallBytes > 0 {
		return decision, &InsufficientError{
			Request:  req,
			Decision: *decision,
		}
	}
	return decision, nil
}

type deviceGroup struct {
	probePath     string
	requiredBytes int64
}

func groupComponentsByDevice(req Request, meter Meter) (map[string]deviceGroup, []string, int64, error) {
	groups := make(map[string]deviceGroup)
	var order []string
	required := int64(0)
	for _, component := range req.Components {
		bytes := component.Bytes
		if bytes < 0 {
			bytes = 0
		}
		if bytes == 0 && strings.TrimSpace(component.Path) == "" {
			continue
		}
		if bytes > 0 {
			required = saturatingAdd(required, bytes)
		}
		probe := logicalProbePath(req, component.Path)
		deviceID, err := componentDeviceID(meter, probe)
		if err != nil {
			return nil, nil, 0, err
		}
		group, ok := groups[deviceID]
		if !ok {
			group = deviceGroup{probePath: probe}
			order = append(order, deviceID)
		}
		group.requiredBytes = saturatingAdd(group.requiredBytes, bytes)
		groups[deviceID] = group
	}
	return groups, order, required, nil
}

func componentDeviceID(meter Meter, probePath string) (string, error) {
	if deviceMeter, ok := meter.(DeviceMeter); ok {
		id, err := deviceMeter.DeviceID(probePath)
		if err != nil {
			return "", fmt.Errorf("check free space device: %w", err)
		}
		if strings.TrimSpace(id) != "" {
			return id, nil
		}
	}
	return filepath.Clean(probePath), nil
}

func (e *InsufficientError) Error() string {
	if e == nil {
		return ""
	}
	return e.message()
}

func (e *InsufficientError) As(target any) bool {
	jvsErr, ok := target.(**errclass.JVSError)
	if !ok {
		return false
	}
	*jvsErr = &errclass.JVSError{Code: ErrCode, Message: e.message()}
	return true
}

func (e *InsufficientError) message() string {
	operation := strings.TrimSpace(e.Request.Operation)
	if operation == "" {
		operation = "operation"
	}
	lines := []string{
		fmt.Sprintf("Not enough free space for %s.", operation),
	}
	if e.Request.Folder != "" {
		lines = append(lines, "Folder: "+e.Request.Folder)
	}
	if e.Request.Workspace != "" {
		lines = append(lines, "Workspace: "+e.Request.Workspace)
	}
	lines = append(lines,
		fmt.Sprintf("Required bytes: %d", e.Decision.RequiredBytes),
		fmt.Sprintf("Safety margin bytes: %d", e.Decision.SafetyMarginBytes),
		fmt.Sprintf("Available bytes: %d", e.Decision.AvailableBytes),
		fmt.Sprintf("Shortfall bytes: %d", e.Decision.ShortfallBytes),
	)
	for _, msg := range e.Request.FailureMessages {
		msg = strings.TrimSpace(msg)
		if msg != "" {
			lines = append(lines, msg)
		}
	}
	return strings.Join(lines, "\n")
}

type StatfsMeter struct{}

func (StatfsMeter) AvailableBytes(path string) (int64, error) {
	probe := existingParent(path)
	var st unix.Statfs_t
	if err := unix.Statfs(probe, &st); err != nil {
		return 0, err
	}
	available, err := availableBytesFromStatfs(st.Bavail, st.Bsize)
	if err != nil {
		return 0, fmt.Errorf("statfs available bytes for %s: %w", probe, err)
	}
	return available, nil
}

func (StatfsMeter) DeviceID(path string) (string, error) {
	probe := existingParent(path)
	var st unix.Stat_t
	if err := unix.Stat(probe, &st); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", st.Dev), nil
}

func probePath(req Request) string {
	for _, component := range req.Components {
		if strings.TrimSpace(component.Path) == "" {
			continue
		}
		return logicalProbePath(req, component.Path)
	}
	if req.Folder != "" {
		return req.Folder
	}
	return "."
}

func logicalProbePath(req Request, path string) string {
	if strings.TrimSpace(path) == "" {
		return probePath(Request{Folder: req.Folder})
	}
	return logicalProbeParent(req.Folder, path)
}

func logicalProbeParent(folder, path string) string {
	clean := filepath.Clean(path)
	if folder != "" {
		control := filepath.Join(folder, ".jvs")
		if rel, err := filepath.Rel(control, clean); err == nil && (rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")) {
			return control
		}
	}
	parent := filepath.Dir(clean)
	if parent == "." || parent == "" {
		return clean
	}
	return parent
}

func existingParent(path string) string {
	clean := filepath.Clean(path)
	for {
		if _, err := os.Stat(clean); err == nil {
			return clean
		}
		parent := filepath.Dir(clean)
		if parent == clean {
			return clean
		}
		clean = parent
	}
}

func saturatingAdd(a, b int64) int64 {
	if b > 0 && a > maxInt64-b {
		return maxInt64
	}
	return a + b
}

func availableBytesFromStatfs(availableBlocks uint64, blockSize int64) (int64, error) {
	if blockSize < 0 {
		return 0, fmt.Errorf("invalid statfs block size %d", blockSize)
	}
	if blockSize == 0 || availableBlocks == 0 {
		return 0, nil
	}
	var blocks big.Int
	blocks.SetUint64(availableBlocks)
	var size big.Int
	size.SetInt64(blockSize)
	var product big.Int
	product.Mul(&blocks, &size)
	if !product.IsInt64() {
		return 0, fmt.Errorf("available space exceeds int64: blocks=%d block_size=%d", availableBlocks, blockSize)
	}
	return product.Int64(), nil
}

const maxInt64 = int64(^uint64(0) >> 1)

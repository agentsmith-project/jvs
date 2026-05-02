package cli

import (
	"fmt"
	"strings"

	"github.com/agentsmith-project/jvs/internal/transfer"
)

func transferDataFromRecord(record *transfer.Record) transfer.Data {
	if record == nil {
		return transfer.Data{Transfers: []transfer.Record{}}
	}
	return transfer.Data{Transfers: []transfer.Record{*record}}
}

func printPrimaryTransferSummary(record *transfer.Record) {
	if record == nil {
		return
	}
	fmt.Printf("Copy method: %s\n", publicCopyMethod(record.PerformanceClass))
	if why := publicTransferWhy(*record); why != "" {
		fmt.Printf("Why: %s\n", why)
	}
	if record.CheckedForThisOperation {
		fmt.Println("Checked for this operation")
	}
}

func printPrimaryExpectedTransferSummary(record *transfer.Record) {
	if record == nil {
		return
	}
	fmt.Printf("Expected copy method: %s\n", publicCopyMethod(record.PerformanceClass))
	if why := publicTransferWhy(*record); why != "" {
		fmt.Printf("Why: %s\n", why)
	}
	if record.CheckedForThisOperation {
		fmt.Println("Checked for this preview")
	}
}

func publicCopyMethod(class transfer.PerformanceClass) string {
	if class == transfer.PerformanceClassFastCopy {
		return "fast copy"
	}
	return "normal copy"
}

func publicTransferWhy(record transfer.Record) string {
	if record.OptimizedTransfer || record.PerformanceClass == transfer.PerformanceClassFastCopy {
		return ""
	}

	reason := firstTransferDetail(record.DegradedReasons, record.Warnings)
	if reason == "" {
		return ""
	}
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "source/destination pair"),
		strings.Contains(lower, "locations cannot use fast copy"),
		strings.Contains(lower, "cross-device"):
		return "these two locations cannot use fast copy together"
	case strings.Contains(lower, "failed"),
		strings.Contains(lower, "fallback"):
		return "fast copy failed during this operation; JVS safely used normal copy"
	case strings.Contains(lower, "unavailable"),
		strings.Contains(lower, "not available"),
		strings.Contains(lower, "not found"),
		strings.Contains(lower, "not on"),
		strings.Contains(lower, "requires writable"):
		return "fast copy was not available for this operation"
	default:
		return "fast copy was not available for this operation"
	}
}

func firstTransferDetail(groups ...[]string) string {
	for _, group := range groups {
		for _, value := range group {
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
}

package cli

import (
	"testing"

	"github.com/agentsmith-project/jvs/internal/transfer"
)

func TestPublicTransferWhySuppressesFastCopyWarnings(t *testing.T) {
	record := transfer.Record{
		OptimizedTransfer: true,
		PerformanceClass:  transfer.PerformanceClassFastCopy,
		Warnings:          []string{"reflink pair probe confidence unknown"},
	}

	if got := publicTransferWhy(record); got != "" {
		t.Fatalf("publicTransferWhy() = %q, want no Why for successful fast copy", got)
	}
}

func TestPublicTransferWhyKeepsNormalCopyDegradation(t *testing.T) {
	tests := []struct {
		name   string
		record transfer.Record
		want   string
	}{
		{
			name: "degraded reason",
			record: transfer.Record{
				OptimizedTransfer: false,
				PerformanceClass:  transfer.PerformanceClassNormalCopy,
				DegradedReasons:   []string{"reflink-copy unavailable for source/destination pair: invalid cross-device link"},
				Warnings:          []string{"reflink pair probe failed: invalid cross-device link"},
			},
			want: "these two locations cannot use fast copy together",
		},
		{
			name: "warning",
			record: transfer.Record{
				OptimizedTransfer: false,
				PerformanceClass:  transfer.PerformanceClassNormalCopy,
				Warnings:          []string{"reflink-copy not available at destination"},
			},
			want: "fast copy was not available for this operation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicTransferWhy(tt.record); got != tt.want {
				t.Fatalf("publicTransferWhy() = %q, want %q", got, tt.want)
			}
		})
	}
}

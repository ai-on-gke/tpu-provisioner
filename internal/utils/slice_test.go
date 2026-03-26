package utils

import (
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

func TestSliceName(t *testing.T) {
	tests := []struct {
		name                 string
		jobSetName           string
		jobSetUID            types.UID
		replicatedJobName    string
		replicatedJobReplica int
		want                 string
	}{
		{
			name:                 "basic slice name",
			jobSetName:           "test-jobset",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "worker",
			replicatedJobReplica: 0,
			want:                 "js-test-jobset-12345678-worker-0",
		},
		{
			name:                 "slice name with long jobset name (should be truncated to 24)",
			jobSetName:           "this-is-a-very-long-jobset-name-that-exceeds-the-limit",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "worker",
			replicatedJobReplica: 0,
			want:                 "js-this-is-a-very-long-jobs-12345678-worker-0",
		},
		{
			name:                 "slice name with long replicated job name (should be truncated to 8)",
			jobSetName:           "test-jobset",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "very-long-replicated-job-name",
			replicatedJobReplica: 0,
			want:                 "js-test-jobset-12345678-very-lon-0",
		},
		{
			name:                 "slice name with high replica index",
			jobSetName:           "test-jobset",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "worker",
			replicatedJobReplica: 99,
			want:                 "js-test-jobset-12345678-worker-99",
		},
		{
			name:                 "slice name with short jobset name",
			jobSetName:           "js",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "w",
			replicatedJobReplica: 5,
			want:                 "js-js-12345678-w-5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SliceName(tt.jobSetName, string(tt.jobSetUID), tt.replicatedJobName, tt.replicatedJobReplica)
			if got != tt.want {
				t.Errorf("SliceName() = %v, want %v", got, tt.want)
			}
			if len(got) > 48 {
				t.Errorf("SliceName() length = %d, want <= 48", len(got))
			}
		})
	}
}

func TestSliceNameMaxLength(t *testing.T) {
	// Worst case: max-length name, long rjName, 2-digit replica
	got := SliceName(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // 36 chars, truncated to 24
		"12345678-1234-1234-1234-123456789012", // full UUID, truncated to 8
		"bbbbbbbbbbbbbbbbbbbb",                 // 20 chars, truncated to 8
		99,                                     // 2-digit replica
	)
	if len(got) > 48 {
		t.Errorf("SliceName() max length = %d, want <= 48; got %q", len(got), got)
	}
}

func TestLWSSliceNameMaxLength(t *testing.T) {
	// Worst case: max-length name, "worker" component, 4-digit replica
	got := LWSSliceName(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // 36 chars, truncated to 23
		"12345678-1234-1234-1234-123456789012", // full UUID, truncated to 8
		"worker",
		9999, // 4-digit replica
	)
	if len(got) > 48 {
		t.Errorf("LWSSliceName() max length = %d, want <= 48; got %q", len(got), got)
	}
}

func TestLegacySliceName(t *testing.T) {
	// Verify legacy names match the old format
	got := LegacySliceName("test-jobset", "12345678-1234", "worker", 0)
	want := "js-test-jobset-12345678-worker-0"
	if got != want {
		t.Errorf("LegacySliceName() = %v, want %v", got, want)
	}
}

func TestLegacyLWSSliceName(t *testing.T) {
	got := LegacyLWSSliceName("test-lws", "12345678-1234", "worker", 0)
	want := "lws-test-lws-12345678-worker-0"
	if got != want {
		t.Errorf("LegacyLWSSliceName() = %v, want %v", got, want)
	}
}

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
			name:                 "slice name with long jobset name (should be truncated)",
			jobSetName:           "this-is-a-very-long-jobset-name-that-exceeds-the-limit",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "worker",
			replicatedJobReplica: 0,
			want:                 "js-this-is-a-very-long-jobset-name--12345678-worker-0",
		},
		{
			name:                 "slice name with long replicated job name (should be truncated)",
			jobSetName:           "test-jobset",
			jobSetUID:            types.UID("12345678-1234"),
			replicatedJobName:    "very-long-replicated-job-name",
			replicatedJobReplica: 0,
			want:                 "js-test-jobset-12345678-very-long--0",
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
				t.Errorf("sliceName() = %v, want %v", got, tt.want)
			}
		})
	}
}

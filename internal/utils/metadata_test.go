package utils

import (
	"testing"
)

func TestIsPartitionIDLabel(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "valid 2x2x1",
			key:  "cloud.google.com/gke-tpu-partition-2x2x1-id",
			want: true,
		},
		{
			name: "valid 4x4x4",
			key:  "cloud.google.com/gke-tpu-partition-4x4x4-id",
			want: true,
		},
		{
			name: "invalid prefix",
			key:  "google.com/gke-tpu-partition-2x2x1-id",
			want: false,
		},
		{
			name: "invalid suffix",
			key:  "cloud.google.com/gke-tpu-partition-2x2x1",
			want: false,
		},
		{
			name: "empty topology",
			key:  "cloud.google.com/gke-tpu-partition--id",
			want: false, // len is 38, check requires > 38
		},
		{
			name: "short string",
			key:  "short",
			want: false,
		},
		{
			name: "valid tricky with extra dashes",
			key:  "cloud.google.com/gke-tpu-partition-custom-topology-id",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPartitionIDLabel(tt.key); got != tt.want {
				t.Errorf("IsPartitionIDLabel(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

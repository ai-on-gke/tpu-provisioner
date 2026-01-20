package utils

import "testing"

func TestSetNodePoolName(t *testing.T) {
	tests := []struct {
		name           string
		nodepoolPrefix string
		blockName      string
		index          int
		want           string
	}{
		{
			name:           "with nodepool prefix",
			nodepoolPrefix: "test-prefix",
			blockName:      "test-block",
			index:          1,
			want:           "test-prefix-0001",
		},
		{
			name:           "without nodepool prefix",
			nodepoolPrefix: "",
			blockName:      "test-block",
			index:          2,
			want:           "test-block-subblock-0002",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SetNodePoolName(tt.nodepoolPrefix, tt.blockName, tt.index); got != tt.want {
				t.Errorf("SetNodePoolName() = %v, want %v", got, tt.want)
			}
		})
	}
}

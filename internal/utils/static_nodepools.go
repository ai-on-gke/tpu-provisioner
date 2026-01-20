package utils

import "fmt"

// SetNodePoolName formats a node pool name for a static nodepool.
func SetNodePoolName(nodepoolPrefix, blockName string, index int) string {
	if nodepoolPrefix != "" {
		return fmt.Sprintf("%s-%04d", nodepoolPrefix, index)
	}
	return fmt.Sprintf("%s-subblock-%04d", blockName, index)
}

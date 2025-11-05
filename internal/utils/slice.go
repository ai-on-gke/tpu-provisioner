package utils

import "fmt"

// SliceName formats a name for a Slice using the following pattern:
// {jobset_name[:32]}-{jobset_uid[:8]}-{replicated_job_name[:10]}-{replicated_job_replica}
// which should result in a max of 3 + 32 + 1 + 8 + 1 + 10 + 1 + 2 = 58 chars.
func SliceName(jsName, jsUID string, replicatedJobName string, replicatedJobReplica int) string {
	return fmt.Sprintf("js-%s-%s-%s-%d",
		jsName[:min(32, len(jsName))],
		jsUID[:8],
		replicatedJobName[:min(10, len(replicatedJobName))],
		replicatedJobReplica,
	)
}

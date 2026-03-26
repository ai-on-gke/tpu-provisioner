package utils

import "fmt"

// SliceName formats a name for a Slice using the following pattern:
// js-{jobset_name[:24]}-{jobset_uid[:8]}-{replicated_job_name[:8]}-{replicated_job_replica}
// Max length with 2-digit replica: 3 + 24 + 1 + 8 + 1 + 8 + 1 + 2 = 48 chars.
func SliceName(jsName, jsUID string, replicatedJobName string, replicatedJobReplica int) string {
	return fmt.Sprintf("js-%s-%s-%s-%d",
		jsName[:min(24, len(jsName))],
		jsUID[:8],
		replicatedJobName[:min(8, len(replicatedJobName))],
		replicatedJobReplica,
	)
}

// LegacySliceName returns the slice name using the old (pre-48-char-limit) format.
// Used for backwards-compatible matching of existing slices.
func LegacySliceName(jsName, jsUID string, replicatedJobName string, replicatedJobReplica int) string {
	return fmt.Sprintf("js-%s-%s-%s-%d",
		jsName[:min(32, len(jsName))],
		jsUID[:8],
		replicatedJobName[:min(10, len(replicatedJobName))],
		replicatedJobReplica,
	)
}

// LWSSliceName formats a name for a Slice owned by a LeaderWorkerSet.
// lws-{lws_name[:23]}-{lws_uid[:8]}-{component}[-{replica}]
// Max length with 4-digit replica: 4 + 23 + 1 + 8 + 1 + 6 + 1 + 4 = 48 chars.
func LWSSliceName(lwsName, lwsUID string, component string, replica int) string {
	name := fmt.Sprintf("lws-%s-%s-%s",
		lwsName[:min(23, len(lwsName))],
		lwsUID[:8],
		component,
	)
	if replica >= 0 {
		name = fmt.Sprintf("%s-%d", name, replica)
	}
	return name
}

// LegacyLWSSliceName returns the LWS slice name using the old (pre-48-char-limit) format.
// Used for backwards-compatible matching of existing slices.
func LegacyLWSSliceName(lwsName, lwsUID string, component string, replica int) string {
	name := fmt.Sprintf("lws-%s-%s-%s",
		lwsName[:min(32, len(lwsName))],
		lwsUID[:8],
		component,
	)
	if replica >= 0 {
		name = fmt.Sprintf("%s-%d", name, replica)
	}
	return name
}

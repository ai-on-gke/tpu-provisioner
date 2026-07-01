package controller

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/ai-on-gke/tpu-provisioner/copied/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

var Now = time.Now

// Labels used to track which resource (i.e. JobSet, LWS) owns a Slice
// since Cluster scopred resources cannot use owner references to Namespaced resources.
const (
	SliceOwnerKindLabel      = "tpu-provisioner.cloud.google.com/owner-kind"
	SliceOwnerNameLabel      = "tpu-provisioner.cloud.google.com/owner-name"
	SliceOwnerNamespaceLabel = "tpu-provisioner.cloud.google.com/owner-namespace"
)

const (
	jobSetOwnerKind = "jobset"
	LWSOwnerKind    = "leaderworkerset"
)

const SliceCleanupFinalizer = "tpu-provisioner.cloud.google.com/slice-cleanup"
const SliceSelectionAnnotation = "tpu-provisioner.cloud.google.com/slice-selection"
const slicePartitionIdsField = "spec.partitionIds"

// SetupSliceFieldIndexer registers the index for PartitionIds to allow quick lookup of slices by partition.
func SetupSliceFieldIndexer(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(context.Background(), &v1beta1.Slice{}, slicePartitionIdsField, func(rawObj client.Object) []string {
		slice := rawObj.(*v1beta1.Slice)
		return slice.Spec.PartitionIds
	})
}

type RecreateCondition struct {
	Reason           string
	MessageSubstring string
}

type diffedSlice struct {
	slice  v1beta1.Slice
	reason string
}

// lookupExistingSlice finds an existing slice by its desired name, falling back to legacy names
// for backwards compatibility. Returns the existing slice and whether it was found.
func lookupExistingSlice(desiredName string, existingMap map[string]*v1beta1.Slice, legacyNames map[string]string) (*v1beta1.Slice, bool) {
	if s, ok := existingMap[desiredName]; ok {
		return s, true
	}
	if legacyName, ok := legacyNames[desiredName]; ok {
		if s, ok := existingMap[legacyName]; ok {
			return s, true
		}
	}
	return nil, false
}

// diffSlices compares desired slices with existing slices and returns
// lists of slices to delete and create. legacyNames maps new desired names to
// their legacy equivalents for backwards-compatible matching of existing slices.
func diffSlices(desired []v1beta1.Slice, existing []v1beta1.Slice, legacyNames map[string]string, now time.Time, recreateConditionReasons []RecreateCondition, conditionalRecreateWait time.Duration) (toDelete, toCreate []diffedSlice, requeueAfter time.Duration) {
	// Create a map of existing slices by name for quick lookup
	existingMap := make(map[string]*v1beta1.Slice)
	for i := range existing {
		existingMap[existing[i].Name] = &existing[i]
	}

	// Check each desired slice
	for _, desiredSlice := range desired {
		if existingSlice, exists := lookupExistingSlice(desiredSlice.Name, existingMap, legacyNames); exists {
			// Slice exists - check if partitions have changed
			if !partitionsEqual(existingSlice.Spec.PartitionIds, desiredSlice.Spec.PartitionIds) {
				// NodeSelector changed - delete existing (creation will happen in next reconcile)
				toDelete = append(toDelete, diffedSlice{slice: *existingSlice, reason: "partition IDs changed"})
				continue
			}

			// Check if slice needs recreation based on its status
			if reason, matches := recreationReasonsMatch(existingSlice, recreateConditionReasons); matches {
				if existingSlice.CreationTimestamp.Add(conditionalRecreateWait).Before(now) {
					toDelete = append(toDelete, diffedSlice{slice: *existingSlice, reason: fmt.Sprintf("recreation condition matched: %s", reason)})
				} else {
					// Jitter between 1 and 3 seconds to prevent thundering herd.
					jitter := time.Duration(1+rand.Intn(2)) * time.Second
					requeueAfter = minDuration(requeueAfter, existingSlice.CreationTimestamp.Add(conditionalRecreateWait).Sub(now)+jitter)
				}
				continue
			}

			// Otherwise, slice matches - no action needed
		} else {
			// Slice doesn't exist - create it
			toCreate = append(toCreate, diffedSlice{slice: desiredSlice, reason: "desired slice does not exist"})
		}
	}

	return toDelete, toCreate, requeueAfter
}

func recreationReasonsMatch(slice *v1beta1.Slice, recreateConditions []RecreateCondition) (string, bool) {
	if len(recreateConditions) == 0 {
		return "", false
	}

	for _, cond := range slice.Status.Conditions {
		if cond.Type == v1beta1.SliceStateConditionType {
			if cond.Status == metav1.ConditionFalse || cond.Status == metav1.ConditionUnknown {
				for _, r := range recreateConditions {
					if cond.Reason == r.Reason && (r.MessageSubstring == "" || strings.Contains(cond.Message, r.MessageSubstring)) {
						reason := cond.Reason
						if cond.Message != "" {
							reason = fmt.Sprintf("%s: %s", cond.Reason, cond.Message)
						}
						return reason, true
					}
				}
			}
		}
	}

	return "", false
}

func ParseRecreateConditions(raw []string) []RecreateCondition {
	var result []RecreateCondition
	for _, s := range raw {
		if s == "" {
			continue
		}
		// Format: Reason or Reason:'Message Substring'
		parts := strings.SplitN(s, ":", 2)
		reason := strings.TrimSpace(parts[0])
		var substring string
		if len(parts) > 1 {
			substring = strings.TrimSpace(parts[1])
			// Strip single quotes if present
			substring = strings.Trim(substring, "'")
		}
		result = append(result, RecreateCondition{
			Reason:           reason,
			MessageSubstring: substring,
		})
	}
	return result
}

// allSlicesReady checks if all desired Slices exist and have the Ready condition set to true.
// legacyNames maps new desired names to their legacy equivalents for backwards compatibility.
func allSlicesReady(desiredSlices []v1beta1.Slice, existingSlices []v1beta1.Slice, legacyNames map[string]string) bool {
	if len(desiredSlices) == 0 {
		return true
	}

	// Create a map of existing slices by name for quick lookup
	existingMap := make(map[string]*v1beta1.Slice)
	for i := range existingSlices {
		existingMap[existingSlices[i].Name] = &existingSlices[i]
	}

	// Check that all desired slices exist and are Ready
	for _, desired := range desiredSlices {
		existing, exists := lookupExistingSlice(desired.Name, existingMap, legacyNames)
		if !exists {
			// Slice doesn't exist yet
			return false
		}
		if !isSliceReady(existing) {
			// Slice exists but is not Ready
			return false
		}
	}

	return true
}

// isSliceReady checks if a Slice has the Ready condition set to true.
func isSliceReady(slice *v1beta1.Slice) bool {
	for _, condition := range slice.Status.Conditions {
		if condition.Type == v1beta1.SliceStateConditionType &&
			condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// countReadySlices returns the number of Slices that have the Ready condition set to true.
func countReadySlices(slices []v1beta1.Slice) int {
	count := 0
	for i := range slices {
		if isSliceReady(&slices[i]) {
			count++
		}
	}
	return count
}

// partitionsEqual compares two partition slices for equality.
func partitionsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// applySliceChanges applies the deletions and creations to the cluster.
func applySliceChanges(ctx context.Context, c client.Client, recorder record.EventRecorder, owner client.Object, toDelete, toCreate []diffedSlice) (time.Duration, error) {
	log := ctrllog.FromContext(ctx)
	var requeueAfter time.Duration

	// Delete slices that have changed
	for _, ds := range toDelete {
		slice := ds.slice
		if slice.DeletionTimestamp != nil {
			log.Info("Skipping deletion of Slice since it is already marked for deletion", "slice", slice.Name, "reason", ds.reason)
			continue
		}
		log.Info("Deleting Slice", "slice", slice.Name, "reason", ds.reason)
		if err := c.Delete(ctx, &slice); err != nil {
			recorder.Eventf(owner, corev1.EventTypeWarning, "SliceDeleteFailed", "Failed to delete Slice %s (reason: %s): %v", slice.Name, ds.reason, err)
			return 0, fmt.Errorf("deleting slice %s: %w", slice.Name, err)
		}
		recorder.Eventf(owner, corev1.EventTypeNormal, "SliceDeleted", "Deleted Slice %s (reason: %s)", slice.Name, ds.reason)
	}

	// Create new slices
	for _, ds := range toCreate {
		slice := ds.slice
		skipped := false
		// Check for overlap with existing Slices in the cluster using the index
		for _, p := range slice.Spec.PartitionIds {
			var overlappingSlices v1beta1.SliceList
			if err := c.List(ctx, &overlappingSlices, client.MatchingFields{slicePartitionIdsField: p}); err != nil {
				return 0, fmt.Errorf("listing slices with partition %s: %w", p, err)
			}

			if len(overlappingSlices.Items) > 0 {
				var names []string
				for _, s := range overlappingSlices.Items {
					names = append(names, s.Name)
				}
				requeueAfter = time.Second
				log.Info("Skipping creation of Slice due to PartitionId overlap with existing Slice(s) in cluster, will requeue",
					"requeueAfter", requeueAfter, "overlappingSliceNames", names)
				recorder.Eventf(owner, corev1.EventTypeWarning, "SliceCreateSkipped", "Skipping creation of Slice %s due to PartitionId overlap with existing Slice(s): %v", slice.Name, names)
				skipped = true
				break
			}
		}

		if skipped {
			continue
		}

		log.Info("Creating Slice", "slice", slice.Name, "reason", ds.reason)
		if err := c.Create(ctx, &slice); err != nil {
			recorder.Eventf(owner, corev1.EventTypeWarning, "SliceCreateFailed", "Failed to create Slice %s (reason: %s): %v", slice.Name, ds.reason, err)
			return 0, fmt.Errorf("creating slice %s: %w", slice.Name, err)
		}
		recorder.Eventf(owner, corev1.EventTypeNormal, "SliceCreated", "Created Slice %s (reason: %s)", slice.Name, ds.reason)
	}

	return requeueAfter, nil
}

// minDuration returns the minimum of two durations, ignoring zero values unless both are zero.
func minDuration(a, b time.Duration) time.Duration {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

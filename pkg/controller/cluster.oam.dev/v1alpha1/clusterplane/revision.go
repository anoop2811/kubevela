/*
Copyright 2025 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clusterplane

import (
	"context"
	"fmt"
	"sort"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
)

const (
	// LabelClusterPlaneName is the label key for ClusterPlane name on revisions
	LabelClusterPlaneName = "cluster.oam.dev/plane-name"

	// LabelClusterPlaneRevisionHash is the label key for revision hash
	LabelClusterPlaneRevisionHash = "cluster.oam.dev/revision-hash"
)

// GeneratePlaneRevision creates a ClusterPlaneRevision from a ClusterPlane
// Returns the revision, whether it's a new revision, and any error
func GeneratePlaneRevision(ctx context.Context, cli client.Client, plane *v1alpha1.ClusterPlane) (*v1alpha1.ClusterPlaneRevision, bool, error) {
	// Get the publish version from annotation
	publishVersion := plane.Annotations[AnnotationPublishVersion]
	if publishVersion == "" {
		return nil, false, fmt.Errorf("publishVersion annotation is required")
	}

	// Generate the revision spec
	revisionSpec := generatePlaneRevisionSpec(plane, publishVersion)

	// Compute hash of the spec
	revisionHash, err := ComputePlaneRevisionHash(&revisionSpec)
	if err != nil {
		return nil, false, fmt.Errorf("failed to compute revision hash: %w", err)
	}

	// Check if we already have a revision with this hash
	existingRev, err := findExistingRevision(ctx, cli, plane, revisionHash)
	if err != nil {
		return nil, false, err
	}
	if existingRev != nil {
		// Revision already exists, no need to create a new one
		return existingRev, false, nil
	}

	// Determine revision number
	revisionNumber, err := getNextRevisionNumber(ctx, cli, plane)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get next revision number: %w", err)
	}

	// Create the revision object
	revisionName := fmt.Sprintf("%s-v%d", plane.Name, revisionNumber)
	revision := &v1alpha1.ClusterPlaneRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revisionName,
			Namespace: plane.Namespace,
			Labels: map[string]string{
				LabelClusterPlaneName:         plane.Name,
				LabelClusterPlaneRevisionHash: revisionHash,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha1.ClusterPlaneGroupVersionKind.GroupVersion().String(),
					Kind:       v1alpha1.ClusterPlaneGroupVersionKind.Kind,
					Name:       plane.Name,
					UID:        plane.UID,
					Controller: ptr.To(true),
				},
			},
		},
		Spec: v1alpha1.ClusterPlaneRevisionSpec{
			PlaneSnapshot: revisionSpec,
			RevisionMeta: v1alpha1.RevisionMeta{
				Created:   metav1.Now(),
				Changelog: plane.Spec.Changelog,
				Digest:    revisionHash,
			},
		},
	}

	// Set parent revision if exists
	if plane.Status.CurrentRevision != nil && plane.Status.CurrentRevision.Name != "" {
		revision.Spec.RevisionMeta.ParentRevision = plane.Status.CurrentRevision.Name
	}

	return revision, true, nil
}

// generatePlaneRevisionSpec creates a PlaneSnapshot from ClusterPlane spec
func generatePlaneRevisionSpec(plane *v1alpha1.ClusterPlane, version string) v1alpha1.PlaneSnapshot {
	snapshot := v1alpha1.PlaneSnapshot{
		Version: version,
	}

	// Deep copy to avoid modifying the original
	if plane.Spec.Owner != nil {
		ownerCopy := *plane.Spec.Owner
		snapshot.Owner = &ownerCopy
	}

	if len(plane.Spec.Components) > 0 {
		snapshot.Components = make([]v1alpha1.PlaneComponent, len(plane.Spec.Components))
		copy(snapshot.Components, plane.Spec.Components)
	}

	if len(plane.Spec.Traits) > 0 {
		snapshot.Traits = make([]v1alpha1.PlaneTrait, len(plane.Spec.Traits))
		copy(snapshot.Traits, plane.Spec.Traits)
	}

	if len(plane.Spec.Policies) > 0 {
		snapshot.Policies = make([]v1alpha1.PlanePolicy, len(plane.Spec.Policies))
		copy(snapshot.Policies, plane.Spec.Policies)
	}

	if len(plane.Spec.Outputs) > 0 {
		snapshot.Outputs = make([]v1alpha1.PlaneOutput, len(plane.Spec.Outputs))
		copy(snapshot.Outputs, plane.Spec.Outputs)
	}

	return snapshot
}

// ComputePlaneRevisionHash computes a hash of the PlaneSnapshot for comparison
func ComputePlaneRevisionHash(snapshot *v1alpha1.PlaneSnapshot) (string, error) {
	return utils.ComputeSpecHash(snapshot)
}

// findExistingRevision looks for an existing revision with the same hash
func findExistingRevision(ctx context.Context, cli client.Client, plane *v1alpha1.ClusterPlane, hash string) (*v1alpha1.ClusterPlaneRevision, error) {
	revisions := &v1alpha1.ClusterPlaneRevisionList{}
	if err := cli.List(ctx, revisions,
		client.InNamespace(plane.Namespace),
		client.MatchingLabels{
			LabelClusterPlaneName:         plane.Name,
			LabelClusterPlaneRevisionHash: hash,
		}); err != nil {
		return nil, fmt.Errorf("failed to list revisions: %w", err)
	}

	if len(revisions.Items) > 0 {
		return &revisions.Items[0], nil
	}
	return nil, nil
}

// getNextRevisionNumber determines the next revision number for a plane
func getNextRevisionNumber(ctx context.Context, cli client.Client, plane *v1alpha1.ClusterPlane) (int64, error) {
	revisions, err := GetPlaneRevisions(ctx, cli, plane.Name, plane.Namespace)
	if err != nil {
		return 0, err
	}

	var maxRevision int64
	for _, rev := range revisions {
		// Extract revision number from name (plane-name-v1 -> 1)
		var revNum int64
		if _, err := fmt.Sscanf(rev.Name, plane.Name+"-v%d", &revNum); err == nil {
			if revNum > maxRevision {
				maxRevision = revNum
			}
		}
	}

	return maxRevision + 1, nil
}

// GetPlaneRevisions returns all revisions for a ClusterPlane
func GetPlaneRevisions(ctx context.Context, cli client.Client, planeName, namespace string) ([]v1alpha1.ClusterPlaneRevision, error) {
	revisionList := &v1alpha1.ClusterPlaneRevisionList{}
	if err := cli.List(ctx, revisionList,
		client.InNamespace(namespace),
		client.MatchingLabels{LabelClusterPlaneName: planeName}); err != nil {
		return nil, err
	}
	return revisionList.Items, nil
}

// GetSortedPlaneRevisions returns revisions sorted by creation time (oldest first)
func GetSortedPlaneRevisions(ctx context.Context, cli client.Client, planeName, namespace string) ([]v1alpha1.ClusterPlaneRevision, error) {
	revisions, err := GetPlaneRevisions(ctx, cli, planeName, namespace)
	if err != nil {
		return nil, err
	}

	sort.Slice(revisions, func(i, j int) bool {
		return revisions[i].CreationTimestamp.Before(&revisions[j].CreationTimestamp)
	})

	return revisions, nil
}

// CreatePlaneRevision creates a new ClusterPlaneRevision
func CreatePlaneRevision(ctx context.Context, cli client.Client, revision *v1alpha1.ClusterPlaneRevision) error {
	// Check if revision already exists
	existing := &v1alpha1.ClusterPlaneRevision{}
	err := cli.Get(ctx, client.ObjectKey{Name: revision.Name, Namespace: revision.Namespace}, existing)
	if err == nil {
		// Revision already exists
		klog.InfoS("ClusterPlaneRevision already exists", "revision", klog.KObj(revision))
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check existing revision: %w", err)
	}

	// Create the revision
	if err := cli.Create(ctx, revision); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create revision: %w", err)
	}

	klog.InfoS("Created ClusterPlaneRevision",
		"revision", klog.KObj(revision),
		"version", revision.Spec.PlaneSnapshot.Version)

	return nil
}

// CleanUpPlaneRevisions removes old revisions beyond the revision limit
func CleanUpPlaneRevisions(ctx context.Context, cli client.Client, plane *v1alpha1.ClusterPlane, revisionLimit int) error {
	revisions, err := GetSortedPlaneRevisions(ctx, cli, plane.Name, plane.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list revisions for cleanup: %w", err)
	}

	// Calculate how many revisions to delete
	// Keep revisionLimit + 1 (current) revisions
	needKill := len(revisions) - revisionLimit - 1
	if needKill <= 0 {
		return nil
	}

	klog.InfoS("Cleaning up old ClusterPlaneRevisions",
		"plane", klog.KRef(plane.Namespace, plane.Name),
		"total", len(revisions),
		"toDelete", needKill,
		"limit", revisionLimit)

	// Get current revision name to avoid deleting it
	var currentRevisionName string
	if plane.Status.CurrentRevision != nil {
		currentRevisionName = plane.Status.CurrentRevision.Name
	}

	// Delete oldest revisions first (they're sorted by creation time)
	deleted := 0
	for _, rev := range revisions {
		if deleted >= needKill {
			break
		}

		// Don't delete the current revision
		if rev.Name == currentRevisionName {
			continue
		}

		if err := cli.Delete(ctx, &rev); err != nil && !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to delete old revision", "revision", klog.KObj(&rev))
			continue
		}

		klog.InfoS("Deleted old ClusterPlaneRevision", "revision", klog.KObj(&rev))
		deleted++
	}

	// Update metric
	metrics.ClusterPlaneRevisionCount.WithLabelValues(plane.Namespace, plane.Name).Set(float64(len(revisions) - deleted))

	return nil
}

// ReconcilePlaneRevision handles the full revision lifecycle
// Returns the revision, whether status was updated, and any error
func ReconcilePlaneRevision(ctx context.Context, cli client.Client, recorder event.Recorder, plane *v1alpha1.ClusterPlane, revisionLimit int) (*v1alpha1.ClusterPlaneRevision, bool, error) {
	// Generate revision
	revision, isNew, err := GeneratePlaneRevision(ctx, cli, plane)
	if err != nil {
		klog.ErrorS(err, "Failed to generate ClusterPlaneRevision", "plane", klog.KObj(plane))
		recorder.Event(plane, event.Warning("RevisionGenerationFailed", err))
		return nil, false, err
	}

	if isNew {
		// Create the revision
		if err := CreatePlaneRevision(ctx, cli, revision); err != nil {
			klog.ErrorS(err, "Failed to create ClusterPlaneRevision", "revision", klog.KObj(revision))
			recorder.Event(plane, event.Warning("RevisionCreationFailed", err))
			return nil, false, err
		}

		recorder.Event(plane, event.Normal("RevisionCreated",
			"Created revision %s with version %s", revision.Name, revision.Spec.PlaneSnapshot.Version))

		// Update revision count metric
		revisions, _ := GetPlaneRevisions(ctx, cli, plane.Name, plane.Namespace)
		metrics.ClusterPlaneRevisionCount.WithLabelValues(plane.Namespace, plane.Name).Set(float64(len(revisions)))

		// Clean up old revisions
		if err := CleanUpPlaneRevisions(ctx, cli, plane, revisionLimit); err != nil {
			klog.ErrorS(err, "Failed to clean up old revisions", "plane", klog.KObj(plane))
			// Don't fail reconciliation for cleanup errors
			recorder.Event(plane, event.Warning("RevisionCleanupFailed", err))
		}
	}

	return revision, isNew, nil
}

// GetRevisionReference creates a RevisionReference from a ClusterPlaneRevision
func GetRevisionReference(revision *v1alpha1.ClusterPlaneRevision) *v1alpha1.RevisionReference {
	return &v1alpha1.RevisionReference{
		Name:    revision.Name,
		Version: revision.Spec.PlaneSnapshot.Version,
	}
}

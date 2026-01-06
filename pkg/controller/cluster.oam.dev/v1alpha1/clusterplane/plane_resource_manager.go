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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

const (
	// ResourceTrackerNamePrefix is the prefix for ResourceTracker names
	ResourceTrackerNamePrefix = "clusterplane-"

	// AnnotationResourceTrackerUID stores the RT UID for verification
	AnnotationResourceTrackerUID = "plane.oam.dev/resource-tracker-uid"
)

// PlaneResourceManager manages Kubernetes resources for a ClusterPlane
// It uses ResourceTracker to track and manage resource ownership
type PlaneResourceManager struct {
	client.Client
}

// NewPlaneResourceManager creates a new PlaneResourceManager
func NewPlaneResourceManager(c client.Client) *PlaneResourceManager {
	return &PlaneResourceManager{
		Client: c,
	}
}

// DispatchResult contains the result of dispatching a resource
type DispatchResult struct {
	// Resource is the resource that was dispatched
	Resource *unstructured.Unstructured

	// Created indicates if the resource was created (vs updated)
	Created bool

	// Updated indicates if the resource was updated
	Updated bool

	// Error contains any error that occurred
	Error error
}

// ComponentDispatchResult contains results for all resources in a component
type ComponentDispatchResult struct {
	// ComponentName is the component that was dispatched
	ComponentName string

	// Results are the individual resource dispatch results
	Results []DispatchResult

	// Error contains any error that prevented dispatching
	Error error
}

// EnsureResourceTracker ensures a ResourceTracker exists for the ClusterPlane
func (m *PlaneResourceManager) EnsureResourceTracker(ctx context.Context, plane *v1alpha1.ClusterPlane) (*corev1beta1.ResourceTracker, error) {
	rtName := GetResourceTrackerName(plane)

	rt := &corev1beta1.ResourceTracker{}
	err := m.Get(ctx, types.NamespacedName{Name: rtName}, rt)
	if err == nil {
		// ResourceTracker exists
		return rt, nil
	}

	if !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get ResourceTracker %s: %w", rtName, err)
	}

	// Create new ResourceTracker
	rt = &corev1beta1.ResourceTracker{
		ObjectMeta: metav1.ObjectMeta{
			Name: rtName,
			Labels: map[string]string{
				LabelPlaneName:      plane.Name,
				LabelPlaneNamespace: plane.Namespace,
				LabelManagedBy:      ManagedByClusterPlane,
			},
			Annotations: map[string]string{
				AnnotationPlaneGeneration: fmt.Sprintf("%d", plane.Generation),
			},
		},
		Spec: corev1beta1.ResourceTrackerSpec{
			Type:             corev1beta1.ResourceTrackerTypeRoot,
			ManagedResources: []corev1beta1.ManagedResource{},
		},
	}

	// Set owner reference so RT is garbage collected when plane is deleted
	rt.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: v1alpha1.SchemeGroupVersion.String(),
			Kind:       "ClusterPlane",
			Name:       plane.Name,
			UID:        plane.UID,
			Controller: boolPtr(true),
		},
	})

	if err := m.Create(ctx, rt); err != nil {
		return nil, fmt.Errorf("failed to create ResourceTracker %s: %w", rtName, err)
	}

	klog.InfoS("Created ResourceTracker for ClusterPlane",
		"resourceTracker", rtName,
		"clusterPlane", klog.KRef(plane.Namespace, plane.Name))

	return rt, nil
}

// GetResourceTracker retrieves the ResourceTracker for a ClusterPlane
func (m *PlaneResourceManager) GetResourceTracker(ctx context.Context, plane *v1alpha1.ClusterPlane) (*corev1beta1.ResourceTracker, error) {
	rtName := GetResourceTrackerName(plane)
	rt := &corev1beta1.ResourceTracker{}
	if err := m.Get(ctx, types.NamespacedName{Name: rtName}, rt); err != nil {
		return nil, err
	}
	return rt, nil
}

// DispatchResources dispatches rendered resources to the cluster
func (m *PlaneResourceManager) DispatchResources(ctx context.Context, plane *v1alpha1.ClusterPlane, resources []*unstructured.Unstructured) ([]DispatchResult, error) {
	// Ensure ResourceTracker exists
	rt, err := m.EnsureResourceTracker(ctx, plane)
	if err != nil {
		return nil, err
	}

	results := make([]DispatchResult, 0, len(resources))
	rtUpdated := false

	for _, resource := range resources {
		result := m.dispatchResource(ctx, resource)
		results = append(results, result)

		// Track the resource in ResourceTracker if dispatch succeeded
		if result.Error == nil {
			if rt.AddManagedResource(resource, false, false, "") {
				rtUpdated = true
			}
		}
	}

	// Update ResourceTracker if resources were added
	if rtUpdated {
		if err := m.Update(ctx, rt); err != nil {
			klog.ErrorS(err, "Failed to update ResourceTracker",
				"resourceTracker", rt.Name,
				"clusterPlane", klog.KRef(plane.Namespace, plane.Name))
			return results, fmt.Errorf("failed to update ResourceTracker: %w", err)
		}
	}

	return results, nil
}

// dispatchResource dispatches a single resource to the cluster
func (m *PlaneResourceManager) dispatchResource(ctx context.Context, resource *unstructured.Unstructured) DispatchResult {
	result := DispatchResult{Resource: resource}

	// Check if resource exists
	existing := resource.DeepCopy()
	err := m.Get(ctx, types.NamespacedName{
		Namespace: resource.GetNamespace(),
		Name:      resource.GetName(),
	}, existing)

	if errors.IsNotFound(err) {
		// Create the resource
		if err := m.Create(ctx, resource); err != nil {
			result.Error = fmt.Errorf("failed to create %s %s/%s: %w",
				resource.GetKind(), resource.GetNamespace(), resource.GetName(), err)
			return result
		}
		result.Created = true
		klog.V(2).InfoS("Created resource",
			"apiVersion", resource.GetAPIVersion(),
			"kind", resource.GetKind(),
			"namespace", resource.GetNamespace(),
			"name", resource.GetName())
		return result
	}

	if err != nil {
		result.Error = fmt.Errorf("failed to get %s %s/%s: %w",
			resource.GetKind(), resource.GetNamespace(), resource.GetName(), err)
		return result
	}

	// Check if resource is managed by this plane
	if !isManagedByPlane(existing, resource) {
		result.Error = fmt.Errorf("resource %s %s/%s exists but is not managed by this ClusterPlane",
			resource.GetKind(), resource.GetNamespace(), resource.GetName())
		return result
	}

	// Update the resource - preserve ResourceVersion for optimistic locking
	resource.SetResourceVersion(existing.GetResourceVersion())
	resource.SetUID(existing.GetUID())

	if err := m.Update(ctx, resource); err != nil {
		result.Error = fmt.Errorf("failed to update %s %s/%s: %w",
			resource.GetKind(), resource.GetNamespace(), resource.GetName(), err)
		return result
	}

	result.Updated = true
	klog.V(2).InfoS("Updated resource",
		"apiVersion", resource.GetAPIVersion(),
		"kind", resource.GetKind(),
		"namespace", resource.GetNamespace(),
		"name", resource.GetName())

	return result
}

// DeleteManagedResources deletes all resources tracked by the ResourceTracker
func (m *PlaneResourceManager) DeleteManagedResources(ctx context.Context, plane *v1alpha1.ClusterPlane) error {
	rt, err := m.GetResourceTracker(ctx, plane)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // No RT means no resources to clean up
		}
		return err
	}

	var deleteErrors []error
	for _, mr := range rt.Spec.ManagedResources {
		if mr.Deleted {
			continue // Already marked for deletion
		}

		u := mr.ToUnstructured()
		if err := m.Delete(ctx, u); err != nil {
			if !errors.IsNotFound(err) {
				deleteErrors = append(deleteErrors, fmt.Errorf("failed to delete %s %s/%s: %w",
					u.GetKind(), u.GetNamespace(), u.GetName(), err))
			}
		} else {
			klog.V(2).InfoS("Deleted managed resource",
				"apiVersion", u.GetAPIVersion(),
				"kind", u.GetKind(),
				"namespace", u.GetNamespace(),
				"name", u.GetName())
		}
	}

	if len(deleteErrors) > 0 {
		return fmt.Errorf("errors deleting managed resources: %v", deleteErrors)
	}

	return nil
}

// GarbageCollect removes resources that are no longer in the desired state
func (m *PlaneResourceManager) GarbageCollect(ctx context.Context, plane *v1alpha1.ClusterPlane, currentResources []*unstructured.Unstructured) error {
	rt, err := m.GetResourceTracker(ctx, plane)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Build a set of current resource keys
	currentKeys := make(map[string]bool)
	for _, r := range currentResources {
		key := resourceKey(r)
		currentKeys[key] = true
	}

	// Find resources to garbage collect
	var toDelete []*unstructured.Unstructured
	rtUpdated := false

	for i, mr := range rt.Spec.ManagedResources {
		u := mr.ToUnstructured()
		key := resourceKey(u)

		if !currentKeys[key] && !mr.SkipGC {
			toDelete = append(toDelete, u)
			// Mark as deleted in RT
			rt.Spec.ManagedResources[i].Deleted = true
			rtUpdated = true
		}
	}

	// Delete stale resources
	for _, u := range toDelete {
		if err := m.Delete(ctx, u); err != nil {
			if !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to garbage collect resource",
					"apiVersion", u.GetAPIVersion(),
					"kind", u.GetKind(),
					"namespace", u.GetNamespace(),
					"name", u.GetName())
			}
		} else {
			klog.InfoS("Garbage collected resource",
				"apiVersion", u.GetAPIVersion(),
				"kind", u.GetKind(),
				"namespace", u.GetNamespace(),
				"name", u.GetName(),
				"clusterPlane", klog.KRef(plane.Namespace, plane.Name))
		}
	}

	// Remove deleted entries from RT
	if rtUpdated {
		var remaining []corev1beta1.ManagedResource
		for _, mr := range rt.Spec.ManagedResources {
			if !mr.Deleted {
				remaining = append(remaining, mr)
			}
		}
		rt.Spec.ManagedResources = remaining

		if err := m.Update(ctx, rt); err != nil {
			return fmt.Errorf("failed to update ResourceTracker after GC: %w", err)
		}
	}

	return nil
}

// GetManagedResources returns all resources managed by this plane
func (m *PlaneResourceManager) GetManagedResources(ctx context.Context, plane *v1alpha1.ClusterPlane) ([]*unstructured.Unstructured, error) {
	rt, err := m.GetResourceTracker(ctx, plane)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	resources := make([]*unstructured.Unstructured, 0, len(rt.Spec.ManagedResources))
	for _, mr := range rt.Spec.ManagedResources {
		if mr.Deleted {
			continue
		}
		resources = append(resources, mr.ToUnstructured())
	}

	return resources, nil
}

// GetResourceTrackerName returns the name of the ResourceTracker for a ClusterPlane
func GetResourceTrackerName(plane *v1alpha1.ClusterPlane) string {
	return fmt.Sprintf("%s%s-%s", ResourceTrackerNamePrefix, plane.Namespace, plane.Name)
}

// resourceKey generates a unique key for a resource
func resourceKey(u *unstructured.Unstructured) string {
	return fmt.Sprintf("%s/%s/%s/%s",
		u.GetAPIVersion(),
		u.GetKind(),
		u.GetNamespace(),
		u.GetName())
}

// isManagedByPlane checks if a resource is managed by the same plane
func isManagedByPlane(existing, desired *unstructured.Unstructured) bool {
	existingLabels := existing.GetLabels()
	desiredLabels := desired.GetLabels()

	if existingLabels == nil {
		return false
	}

	// Check if managed by ClusterPlane
	if existingLabels[LabelManagedBy] != ManagedByClusterPlane {
		return false
	}

	// Check if same plane
	if existingLabels[LabelPlaneName] != desiredLabels[LabelPlaneName] {
		return false
	}
	if existingLabels[LabelPlaneNamespace] != desiredLabels[LabelPlaneNamespace] {
		return false
	}

	return true
}

func boolPtr(b bool) *bool {
	return &b
}

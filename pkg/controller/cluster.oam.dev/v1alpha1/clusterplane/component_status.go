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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
)

// InitializeComponentStatuses initializes the component health statuses from spec components.
// This ensures all components have a status entry, even if just Pending.
func InitializeComponentStatuses(plane *v1alpha1.ClusterPlane) {
	if len(plane.Spec.Components) == 0 {
		plane.Status.ComponentHealth = nil
		plane.Status.HealthSummary = nil
		return
	}

	// Build a map of existing component statuses for quick lookup
	existingStatuses := make(map[string]v1alpha1.ComponentHealthStatus)
	for _, cs := range plane.Status.ComponentHealth {
		existingStatuses[cs.Name] = cs
	}

	now := metav1.Now().Format(time.RFC3339)
	newStatuses := make([]v1alpha1.ComponentHealthStatus, 0, len(plane.Spec.Components))

	for _, comp := range plane.Spec.Components {
		if existing, found := existingStatuses[comp.Name]; found {
			// Preserve existing status but update type if changed
			existing.Type = comp.Type
			newStatuses = append(newStatuses, existing)
		} else {
			// Initialize new component with Pending phase
			newStatuses = append(newStatuses, v1alpha1.ComponentHealthStatus{
				Name:               comp.Name,
				Type:               comp.Type,
				Phase:              v1alpha1.ComponentPhasePending,
				Healthy:            false,
				Message:            "Component waiting to be deployed",
				Reason:             "Pending",
				LastTransitionTime: &now,
			})
		}
	}

	plane.Status.ComponentHealth = newStatuses
}

// UpdateComponentStatus updates the status of a specific component.
// Returns true if the status changed.
func UpdateComponentStatus(
	plane *v1alpha1.ClusterPlane,
	componentName string,
	phase v1alpha1.PlaneComponentPhase,
	healthy bool,
	message string,
	reason string,
) bool {
	now := metav1.Now().Format(time.RFC3339)

	for i := range plane.Status.ComponentHealth {
		if plane.Status.ComponentHealth[i].Name == componentName {
			cs := &plane.Status.ComponentHealth[i]

			// Check if status actually changed
			if cs.Phase == phase && cs.Healthy == healthy && cs.Reason == reason {
				return false
			}

			// Update the status
			cs.Phase = phase
			cs.Healthy = healthy
			cs.Message = message
			cs.Reason = reason
			cs.LastTransitionTime = &now
			return true
		}
	}
	return false
}

// SetComponentDeploying marks a component as deploying
func SetComponentDeploying(plane *v1alpha1.ClusterPlane, componentName string) bool {
	return UpdateComponentStatus(plane, componentName,
		v1alpha1.ComponentPhaseDeploying, false, "Component is being deployed", "Deploying")
}

// SetComponentRunning marks a component as running and healthy
func SetComponentRunning(plane *v1alpha1.ClusterPlane, componentName string) bool {
	return UpdateComponentStatus(plane, componentName,
		v1alpha1.ComponentPhaseRunning, true, "Component is running and healthy", "Healthy")
}

// SetComponentDegraded marks a component as degraded (running but not fully healthy)
func SetComponentDegraded(plane *v1alpha1.ClusterPlane, componentName, message string) bool {
	return UpdateComponentStatus(plane, componentName,
		v1alpha1.ComponentPhaseDegraded, false, message, "Degraded")
}

// SetComponentFailed marks a component as failed
func SetComponentFailed(plane *v1alpha1.ClusterPlane, componentName, message string) bool {
	return UpdateComponentStatus(plane, componentName,
		v1alpha1.ComponentPhaseFailed, false, message, "Failed")
}

// CalculateHealthSummary calculates the aggregate health summary from component statuses
func CalculateHealthSummary(plane *v1alpha1.ClusterPlane) *v1alpha1.PlaneHealthSummary {
	if len(plane.Status.ComponentHealth) == 0 {
		return nil
	}

	summary := &v1alpha1.PlaneHealthSummary{
		TotalComponents: len(plane.Status.ComponentHealth),
	}

	for _, cs := range plane.Status.ComponentHealth {
		switch cs.Phase {
		case v1alpha1.ComponentPhaseRunning:
			if cs.Healthy {
				summary.HealthyComponents++
			} else {
				summary.DegradedComponents++
			}
		case v1alpha1.ComponentPhaseDegraded:
			summary.DegradedComponents++
		case v1alpha1.ComponentPhaseFailed:
			summary.FailedComponents++
		case v1alpha1.ComponentPhasePending, v1alpha1.ComponentPhaseDeploying:
			summary.PendingComponents++
		default:
			// Unknown or other phases count as pending
			summary.PendingComponents++
		}
	}

	// Overall healthy if all components are healthy and none are failed/degraded/pending
	summary.OverallHealthy = summary.HealthyComponents == summary.TotalComponents &&
		summary.FailedComponents == 0 &&
		summary.DegradedComponents == 0 &&
		summary.PendingComponents == 0

	return summary
}

// DeterminePhaseFromHealth determines the plane phase based on component health.
// This should be called after CalculateHealthSummary.
func DeterminePhaseFromHealth(plane *v1alpha1.ClusterPlane) v1alpha1.PlanePhase {
	summary := plane.Status.HealthSummary
	if summary == nil {
		// No health summary, keep current phase or return Draft
		if plane.Status.Phase != "" {
			return plane.Status.Phase
		}
		return v1alpha1.PlanePhaseDraft
	}

	// If any components failed, the plane is failed
	if summary.FailedComponents > 0 {
		return v1alpha1.PlanePhaseFailed
	}

	// If any components are degraded, the plane is degraded
	if summary.DegradedComponents > 0 {
		return v1alpha1.PlanePhaseDegraded
	}

	// If any components are pending, we're still working
	if summary.PendingComponents > 0 {
		return v1alpha1.PlanePhasePublishing
	}

	// All components healthy
	if summary.OverallHealthy {
		return v1alpha1.PlanePhaseRunning
	}

	return v1alpha1.PlanePhaseRunning
}

// ReconcileComponentStatuses performs a full status reconciliation:
// 1. Initializes statuses for new components
// 2. Calculates health summary
// 3. Updates the plane phase based on health
func ReconcileComponentStatuses(plane *v1alpha1.ClusterPlane) {
	// Initialize statuses for all spec components
	InitializeComponentStatuses(plane)

	// Calculate aggregate health
	plane.Status.HealthSummary = CalculateHealthSummary(plane)
}

// GetComponentStatus returns the status of a specific component, or nil if not found
func GetComponentStatus(plane *v1alpha1.ClusterPlane, componentName string) *v1alpha1.ComponentHealthStatus {
	for i := range plane.Status.ComponentHealth {
		if plane.Status.ComponentHealth[i].Name == componentName {
			return &plane.Status.ComponentHealth[i]
		}
	}
	return nil
}

// SetTraitStatus updates or adds a trait status for a component
func SetTraitStatus(plane *v1alpha1.ClusterPlane, componentName, traitType string, healthy bool, message string) {
	cs := GetComponentStatus(plane, componentName)
	if cs == nil {
		return
	}

	// Find existing trait status
	for i := range cs.TraitStatuses {
		if cs.TraitStatuses[i].Type == traitType {
			cs.TraitStatuses[i].Healthy = healthy
			cs.TraitStatuses[i].Message = message
			return
		}
	}

	// Add new trait status
	cs.TraitStatuses = append(cs.TraitStatuses, v1alpha1.TraitHealthStatus{
		Type:    traitType,
		Healthy: healthy,
		Message: message,
	})
}

// RecalculateHealthSummary recalculates and updates the health summary
// This is useful after updating individual component statuses
func RecalculateHealthSummary(plane *v1alpha1.ClusterPlane) {
	plane.Status.HealthSummary = CalculateHealthSummary(plane)
}

// SetComponentDetail sets a detail key-value pair for a component
func SetComponentDetail(plane *v1alpha1.ClusterPlane, componentName, key, value string) {
	cs := GetComponentStatus(plane, componentName)
	if cs == nil {
		return
	}

	if cs.Details == nil {
		cs.Details = make(map[string]string)
	}
	cs.Details[key] = value
}

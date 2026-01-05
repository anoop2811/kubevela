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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

// ClusterPlaneSpec defines the desired state of ClusterPlane
type ClusterPlaneSpec struct {
	// Description provides documentation for this plane
	// +optional
	Description string `json:"description,omitempty"`

	// Changelog documents changes for this version
	// +optional
	Changelog string `json:"changelog,omitempty"`

	// Owner specifies the team that owns this plane
	// +optional
	Owner *OwnerInfo `json:"owner,omitempty"`

	// Components define the infrastructure components in this plane
	// +optional
	Components []PlaneComponent `json:"components,omitempty"`

	// Traits define plane-level traits applied to all components
	// +optional
	Traits []PlaneTrait `json:"traits,omitempty"`

	// Policies define plane-level policies
	// +optional
	Policies []PlanePolicy `json:"policies,omitempty"`

	// Outputs define values exported by this plane for cross-plane references
	// +optional
	Outputs []PlaneOutput `json:"outputs,omitempty"`

	// CrossClusterInputs define dependencies on other planes in different clusters
	// +optional
	CrossClusterInputs []CrossClusterInput `json:"crossClusterInputs,omitempty"`

	// RevisionHistoryLimit specifies the maximum number of revisions to retain
	// +kubebuilder:default=10
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// ClusterPlaneStatus defines the observed state of ClusterPlane
type ClusterPlaneStatus struct {
	// ConditionedStatus provides standard condition fields
	condition.ConditionedStatus `json:",inline"`

	// Phase represents the current lifecycle phase of the plane
	// +optional
	Phase PlanePhase `json:"phase,omitempty"`

	// CurrentRevision references the currently active revision
	// +optional
	CurrentRevision *RevisionReference `json:"currentRevision,omitempty"`

	// RevisionCount is the total number of revisions created
	// +optional
	RevisionCount int32 `json:"revisionCount,omitempty"`

	// ObservedGeneration is the generation last observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ComponentHealth summarizes the health of each component
	// +optional
	ComponentHealth []ComponentHealthStatus `json:"componentHealth,omitempty"`

	// HealthSummary provides an aggregate view of plane health
	// +optional
	HealthSummary *PlaneHealthSummary `json:"healthSummary,omitempty"`

	// Outputs contains the resolved output values from this plane
	// +optional
	Outputs map[string]string `json:"outputs,omitempty"`

	// ResolvedOutputs tracks the status of output resolution
	// +optional
	ResolvedOutputs []ResolvedOutputStatus `json:"resolvedOutputs,omitempty"`

	// OutputResolutionSummary provides an aggregate view of output resolution
	// +optional
	OutputResolutionSummary *OutputResolutionSummary `json:"outputResolutionSummary,omitempty"`

	// ResolvedInputs tracks the status of cross-cluster input resolution
	// +optional
	ResolvedInputs []ResolvedInputStatus `json:"resolvedInputs,omitempty"`

	// InputResolutionSummary provides an aggregate view of input resolution
	// +optional
	InputResolutionSummary *InputResolutionSummary `json:"inputResolutionSummary,omitempty"`

	// ResourceTrackerRef references the ResourceTracker for this plane
	// +optional
	ResourceTrackerRef *ResourceTrackerReference `json:"resourceTrackerRef,omitempty"`

	// LastUpdated is the timestamp of the last status update
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={oam,cluster},shortName=cp
// +kubebuilder:printcolumn:name="PHASE",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="REVISION",type=string,JSONPath=`.status.currentRevision.version`
// +kubebuilder:printcolumn:name="HEALTHY",type=string,JSONPath=`.status.healthSummary.overallHealthy`
// +kubebuilder:printcolumn:name="COMPONENTS",type=integer,JSONPath=`.status.healthSummary.totalComponents`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterPlane represents a composable infrastructure layer for cluster management.
// A ClusterPlane contains components, traits, and policies that define a logical
// unit of cluster infrastructure (e.g., networking plane, security plane).
// ClusterPlanes can be versioned and composed into ClusterBlueprints.
type ClusterPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterPlaneSpec   `json:"spec,omitempty"`
	Status ClusterPlaneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterPlaneList contains a list of ClusterPlane
type ClusterPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterPlane `json:"items"`
}

// SetConditions sets the conditions on the ClusterPlane status
func (cp *ClusterPlane) SetConditions(c ...condition.Condition) {
	cp.Status.SetConditions(c...)
}

// GetCondition gets a specific condition from the ClusterPlane status
func (cp *ClusterPlane) GetCondition(ct condition.ConditionType) condition.Condition {
	return cp.Status.GetCondition(ct)
}

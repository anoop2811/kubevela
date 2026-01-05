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
)

// PlaneSnapshot contains an immutable snapshot of the ClusterPlane spec
type PlaneSnapshot struct {
	// Version is the semantic version of this snapshot
	Version string `json:"version"`

	// Owner contains ownership information at snapshot time
	// +optional
	Owner *OwnerInfo `json:"owner,omitempty"`

	// Components is the snapshot of components at this version
	// +optional
	Components []PlaneComponent `json:"components,omitempty"`

	// Traits is the snapshot of plane-level traits
	// +optional
	Traits []PlaneTrait `json:"traits,omitempty"`

	// Policies is the snapshot of plane-level policies
	// +optional
	Policies []PlanePolicy `json:"policies,omitempty"`

	// Outputs is the snapshot of defined outputs
	// +optional
	Outputs []PlaneOutput `json:"outputs,omitempty"`
}

// RevisionMeta contains metadata about the revision
type RevisionMeta struct {
	// Created is when this revision was created
	Created metav1.Time `json:"created"`

	// CreatedBy identifies who created this revision
	// +optional
	CreatedBy string `json:"createdBy,omitempty"`

	// Changelog describes the changes in this revision
	// +optional
	Changelog string `json:"changelog,omitempty"`

	// Digest is the SHA256 hash of the spec for integrity verification
	Digest string `json:"digest"`

	// ParentRevision is the name of the previous revision (for diff)
	// +optional
	ParentRevision string `json:"parentRevision,omitempty"`
}

// CompressionSpec defines compression settings for large specs
type CompressionSpec struct {
	// Type is the compression algorithm (gzip, zstd, none)
	// +kubebuilder:validation:Enum=gzip;zstd;none
	// +kubebuilder:default=none
	Type string `json:"type,omitempty"`
}

// ClusterPlaneRevisionSpec defines the desired state of ClusterPlaneRevision
type ClusterPlaneRevisionSpec struct {
	// PlaneSnapshot is the immutable snapshot of the ClusterPlane spec
	PlaneSnapshot PlaneSnapshot `json:"planeSnapshot"`

	// RevisionMeta contains metadata about this revision
	RevisionMeta RevisionMeta `json:"revisionMeta"`

	// Compression specifies compression settings for storage
	// +optional
	Compression *CompressionSpec `json:"compression,omitempty"`
}

// ClusterPlaneRevisionStatus defines the observed state of ClusterPlaneRevision
type ClusterPlaneRevisionStatus struct {
	// Succeeded indicates if this revision was successfully created
	// +optional
	Succeeded bool `json:"succeeded,omitempty"`

	// ActiveInClusters lists clusters currently using this revision
	// +optional
	ActiveInClusters []ClusterReference `json:"activeInClusters,omitempty"`

	// Outputs contains the resolved output values from this revision
	// +optional
	Outputs map[string]string `json:"outputs,omitempty"`

	// ResourceTrackerRef references the ResourceTracker for garbage collection
	// +optional
	ResourceTrackerRef *ResourceTrackerReference `json:"resourceTrackerRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={oam,cluster},shortName=cpr
// +kubebuilder:printcolumn:name="PLANE",type=string,JSONPath=`.metadata.labels['cluster\.oam\.dev/plane-name']`
// +kubebuilder:printcolumn:name="VERSION",type=string,JSONPath=`.spec.planeSnapshot.version`
// +kubebuilder:printcolumn:name="SUCCEEDED",type=boolean,JSONPath=`.status.succeeded`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterPlaneRevision is an immutable snapshot of a ClusterPlane at a specific version.
// Revisions are created when a ClusterPlane has the plane.oam.dev/publishVersion annotation set.
// This follows the same pattern as ApplicationRevision for Applications.
type ClusterPlaneRevision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterPlaneRevisionSpec   `json:"spec,omitempty"`
	Status ClusterPlaneRevisionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterPlaneRevisionList contains a list of ClusterPlaneRevision
type ClusterPlaneRevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterPlaneRevision `json:"items"`
}

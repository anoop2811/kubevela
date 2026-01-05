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
	"k8s.io/apimachinery/pkg/runtime"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
)

// PlanePhase is a label for the condition of a ClusterPlane at the current time
type PlanePhase string

const (
	// PlanePhaseDraft means the plane is in draft mode (no publishVersion annotation)
	PlanePhaseDraft PlanePhase = "Draft"

	// PlanePhasePublishing means the plane is being published (creating revision)
	PlanePhasePublishing PlanePhase = "Publishing"

	// PlanePhaseRunning means the plane is actively being used
	PlanePhaseRunning PlanePhase = "Running"

	// PlanePhaseSuspended means the plane reconciliation is suspended
	PlanePhaseSuspended PlanePhase = "Suspended"

	// PlanePhaseFailed means the plane encountered an error
	PlanePhaseFailed PlanePhase = "Failed"
)

// PlaneComponent defines a component within a ClusterPlane
// Similar to ApplicationComponent but tailored for cluster infrastructure
type PlaneComponent struct {
	// Name is the unique identifier of this component within the plane
	Name string `json:"name"`

	// Type refers to a PlaneComponentDefinition
	Type string `json:"type"`

	// Properties are the configuration for this component
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties *runtime.RawExtension `json:"properties,omitempty"`

	// DependsOn specifies components that must be healthy before this one is deployed
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// Inputs define values to be read from other components or planes
	// +optional
	Inputs workflowv1alpha1.StepInputs `json:"inputs,omitempty"`

	// Outputs define values to be exported for other components or planes
	// +optional
	Outputs workflowv1alpha1.StepOutputs `json:"outputs,omitempty"`

	// Traits define operational behaviors attached to this component
	// +optional
	Traits []PlaneTrait `json:"traits,omitempty"`
}

// PlaneTrait defines a trait attached to a PlaneComponent
type PlaneTrait struct {
	// Type refers to a PlaneTraitDefinition
	Type string `json:"type"`

	// Properties are the configuration for this trait
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties *runtime.RawExtension `json:"properties,omitempty"`
}

// PlanePolicy defines a policy applied to the entire plane
type PlanePolicy struct {
	// Name is the unique identifier of this policy within the plane
	// +optional
	Name string `json:"name,omitempty"`

	// Type refers to a PlanePolicyDefinition
	Type string `json:"type"`

	// Properties are the configuration for this policy
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties *runtime.RawExtension `json:"properties,omitempty"`
}

// CrossClusterInput defines a dependency on another plane in a different cluster
type CrossClusterInput struct {
	// Name is the identifier for this input (used in templating)
	Name string `json:"name"`

	// FromCluster specifies the source cluster name
	FromCluster string `json:"fromCluster"`

	// FromPlane specifies the source ClusterPlane name
	FromPlane string `json:"fromPlane"`

	// FromNamespace specifies the namespace of the source ClusterPlane
	// +optional
	FromNamespace string `json:"fromNamespace,omitempty"`

	// Output specifies which output from the source plane to read
	Output string `json:"output"`

	// Required indicates if this input must resolve for the plane to proceed
	// +kubebuilder:default=true
	Required bool `json:"required"`

	// FallbackValue is used if the input cannot be resolved and Required is false
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	FallbackValue *runtime.RawExtension `json:"fallbackValue,omitempty"`
}

// PlaneOutput defines a value exported by the plane for cross-plane references
type PlaneOutput struct {
	// Name is the identifier for this output
	Name string `json:"name"`

	// ValueFrom specifies how to derive the output value
	ValueFrom PlaneOutputValueFrom `json:"valueFrom"`
}

// PlaneOutputValueFrom specifies how to derive an output value
type PlaneOutputValueFrom struct {
	// Component is the name of the component to read from
	Component string `json:"component"`

	// FieldPath is the path to the field in the component's status
	FieldPath string `json:"fieldPath"`
}

// OwnerInfo contains information about the team that owns this plane
type OwnerInfo struct {
	// Team is the name of the owning team
	Team string `json:"team"`

	// Contacts is a list of contact emails for the team
	// +optional
	Contacts []string `json:"contacts,omitempty"`
}

// ResourceTrackerReference contains a reference to a ResourceTracker
type ResourceTrackerReference struct {
	// Name is the name of the ResourceTracker
	Name string `json:"name"`

	// UID is the unique identifier of the ResourceTracker
	// +optional
	UID string `json:"uid,omitempty"`
}

// RevisionReference contains a reference to a revision
type RevisionReference struct {
	// Name is the full name of the revision (e.g., networking-v2.3.1)
	Name string `json:"name"`

	// Version is the semantic version (e.g., 2.3.1)
	Version string `json:"version"`
}

// ClusterReference contains information about a cluster using this plane/revision
type ClusterReference struct {
	// Name is the cluster name
	Name string `json:"name"`

	// SyncedAt is when this revision was last synced to the cluster
	// +optional
	SyncedAt string `json:"syncedAt,omitempty"`
}

// ComponentHealthStatus represents the health status of a component
type ComponentHealthStatus struct {
	// Name is the component name
	Name string `json:"name"`

	// Healthy indicates if the component is healthy
	Healthy bool `json:"healthy"`

	// Message provides additional health information
	// +optional
	Message string `json:"message,omitempty"`
}

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
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ClusterPlane type metadata
var (
	ClusterPlaneKind             = reflect.TypeOf(ClusterPlane{}).Name()
	ClusterPlaneGroupKind        = schema.GroupKind{Group: Group, Kind: ClusterPlaneKind}.String()
	ClusterPlaneKindAPIVersion   = ClusterPlaneKind + "." + SchemeGroupVersion.String()
	ClusterPlaneGroupVersionKind = SchemeGroupVersion.WithKind(ClusterPlaneKind)
	ClusterPlaneGVR              = SchemeGroupVersion.WithResource("clusterplanes")
)

// ClusterPlaneRevision type metadata
var (
	ClusterPlaneRevisionKind             = reflect.TypeOf(ClusterPlaneRevision{}).Name()
	ClusterPlaneRevisionGroupKind        = schema.GroupKind{Group: Group, Kind: ClusterPlaneRevisionKind}.String()
	ClusterPlaneRevisionKindAPIVersion   = ClusterPlaneRevisionKind + "." + SchemeGroupVersion.String()
	ClusterPlaneRevisionGroupVersionKind = SchemeGroupVersion.WithKind(ClusterPlaneRevisionKind)
	ClusterPlaneRevisionGVR              = SchemeGroupVersion.WithResource("clusterplanerevisions")
)

func init() {
	SchemeBuilder.Register(&ClusterPlane{}, &ClusterPlaneList{})
	SchemeBuilder.Register(&ClusterPlaneRevision{}, &ClusterPlaneRevisionList{})
}

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
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
)

const (
	// LabelPlaneName is the label key for the ClusterPlane name
	LabelPlaneName = "plane.oam.dev/name"

	// LabelPlaneNamespace is the label key for the ClusterPlane namespace
	LabelPlaneNamespace = "plane.oam.dev/namespace"

	// LabelPlaneComponent is the label key for the component name within a plane
	LabelPlaneComponent = "plane.oam.dev/component"

	// LabelPlaneRevision is the label key for the revision this resource belongs to
	LabelPlaneRevision = "plane.oam.dev/revision"

	// LabelPlaneComponentType is the label key for the component type
	LabelPlaneComponentType = "plane.oam.dev/component-type"

	// LabelManagedBy is the standard managed-by label
	LabelManagedBy = "app.kubernetes.io/managed-by"

	// ManagedByClusterPlane is the value for LabelManagedBy
	ManagedByClusterPlane = "kubevela-clusterplane"

	// AnnotationPlaneGeneration is the annotation key for the plane generation
	AnnotationPlaneGeneration = "plane.oam.dev/generation"

	// ComponentTypeRaw represents a raw Kubernetes resource component
	ComponentTypeRaw = "raw"

	// ComponentTypeKubernetes is an alias for raw
	ComponentTypeKubernetes = "kubernetes"

	// ComponentTypeHelm represents a Helm chart component (future)
	ComponentTypeHelm = "helm"

	// ComponentTypeKustomize represents a Kustomize component (future)
	ComponentTypeKustomize = "kustomize"
)

// ComponentRenderer renders PlaneComponents into Kubernetes resources
type ComponentRenderer interface {
	// Render renders a PlaneComponent into a list of unstructured resources
	Render(ctx context.Context, plane *v1alpha1.ClusterPlane, component *v1alpha1.PlaneComponent) ([]*unstructured.Unstructured, error)

	// SupportsType returns true if this renderer supports the given component type
	SupportsType(componentType string) bool
}

// RenderResult contains the result of rendering a component
type RenderResult struct {
	// ComponentName is the name of the component that was rendered
	ComponentName string

	// ComponentType is the type of the component
	ComponentType string

	// Resources are the rendered Kubernetes resources
	Resources []*unstructured.Unstructured

	// Error contains any error that occurred during rendering
	Error error
}

// CompositeRenderer manages multiple component renderers
type CompositeRenderer struct {
	renderers []ComponentRenderer
}

// NewCompositeRenderer creates a new CompositeRenderer with default renderers
func NewCompositeRenderer() *CompositeRenderer {
	return &CompositeRenderer{
		renderers: []ComponentRenderer{
			NewRawComponentRenderer(),
		},
	}
}

// RegisterRenderer adds a renderer to the composite
func (cr *CompositeRenderer) RegisterRenderer(r ComponentRenderer) {
	cr.renderers = append(cr.renderers, r)
}

// Render renders a component using the appropriate renderer
func (cr *CompositeRenderer) Render(ctx context.Context, plane *v1alpha1.ClusterPlane, component *v1alpha1.PlaneComponent) ([]*unstructured.Unstructured, error) {
	for _, renderer := range cr.renderers {
		if renderer.SupportsType(component.Type) {
			return renderer.Render(ctx, plane, component)
		}
	}
	return nil, fmt.Errorf("no renderer found for component type %q", component.Type)
}

// RenderAll renders all components in a ClusterPlane
func (cr *CompositeRenderer) RenderAll(ctx context.Context, plane *v1alpha1.ClusterPlane) []RenderResult {
	results := make([]RenderResult, 0, len(plane.Spec.Components))

	for i := range plane.Spec.Components {
		component := &plane.Spec.Components[i]
		resources, err := cr.Render(ctx, plane, component)
		results = append(results, RenderResult{
			ComponentName: component.Name,
			ComponentType: component.Type,
			Resources:     resources,
			Error:         err,
		})
	}

	return results
}

// RawComponentRenderer renders "raw" and "kubernetes" type components
// These components contain raw Kubernetes resource definitions in their Properties
type RawComponentRenderer struct{}

// NewRawComponentRenderer creates a new RawComponentRenderer
func NewRawComponentRenderer() *RawComponentRenderer {
	return &RawComponentRenderer{}
}

// SupportsType returns true for "raw" and "kubernetes" component types
func (r *RawComponentRenderer) SupportsType(componentType string) bool {
	return componentType == ComponentTypeRaw || componentType == ComponentTypeKubernetes
}

// Render renders a raw component's Properties as Kubernetes resources
func (r *RawComponentRenderer) Render(ctx context.Context, plane *v1alpha1.ClusterPlane, component *v1alpha1.PlaneComponent) ([]*unstructured.Unstructured, error) {
	if component.Properties == nil || len(component.Properties.Raw) == 0 {
		return nil, fmt.Errorf("component %q has no properties", component.Name)
	}

	// Try to unmarshal as a single resource first
	var resources []*unstructured.Unstructured

	// Check if Properties is an array or a single object
	var rawData interface{}
	if err := json.Unmarshal(component.Properties.Raw, &rawData); err != nil {
		return nil, fmt.Errorf("failed to parse component %q properties: %w", component.Name, err)
	}

	switch data := rawData.(type) {
	case []interface{}:
		// It's an array of resources
		for i, item := range data {
			obj, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("component %q properties[%d] is not an object", component.Name, i)
			}
			u := &unstructured.Unstructured{Object: obj}
			r.addPlaneLabels(u, plane, component)
			resources = append(resources, u)
		}
	case map[string]interface{}:
		// Check if it's a List type (apiVersion + kind + items)
		if items, hasItems := data["items"]; hasItems {
			if itemsList, ok := items.([]interface{}); ok {
				for i, item := range itemsList {
					obj, ok := item.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("component %q items[%d] is not an object", component.Name, i)
					}
					u := &unstructured.Unstructured{Object: obj}
					r.addPlaneLabels(u, plane, component)
					resources = append(resources, u)
				}
			}
		} else {
			// It's a single resource
			u := &unstructured.Unstructured{Object: data}
			r.addPlaneLabels(u, plane, component)
			resources = append(resources, u)
		}
	default:
		return nil, fmt.Errorf("component %q properties has unsupported format", component.Name)
	}

	// Validate each resource has required fields
	for i, u := range resources {
		if u.GetAPIVersion() == "" {
			return nil, fmt.Errorf("component %q resource[%d] missing apiVersion", component.Name, i)
		}
		if u.GetKind() == "" {
			return nil, fmt.Errorf("component %q resource[%d] missing kind", component.Name, i)
		}
		// If no name is specified, generate one from component name
		if u.GetName() == "" {
			if len(resources) == 1 {
				u.SetName(component.Name)
			} else {
				u.SetName(fmt.Sprintf("%s-%d", component.Name, i))
			}
		}
		// Default namespace to plane namespace if not specified and not cluster-scoped
		if u.GetNamespace() == "" && isNamespaced(u) {
			u.SetNamespace(plane.Namespace)
		}
	}

	return resources, nil
}

// addPlaneLabels adds standard ClusterPlane labels to a resource
func (r *RawComponentRenderer) addPlaneLabels(u *unstructured.Unstructured, plane *v1alpha1.ClusterPlane, component *v1alpha1.PlaneComponent) {
	labels := u.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	labels[LabelPlaneName] = plane.Name
	labels[LabelPlaneNamespace] = plane.Namespace
	labels[LabelPlaneComponent] = component.Name
	labels[LabelPlaneComponentType] = component.Type
	labels[LabelManagedBy] = ManagedByClusterPlane

	// Add revision label if plane has a current revision
	if plane.Status.CurrentRevision != nil && plane.Status.CurrentRevision.Name != "" {
		labels[LabelPlaneRevision] = plane.Status.CurrentRevision.Name
	}

	u.SetLabels(labels)

	// Add generation annotation for tracking
	annotations := u.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[AnnotationPlaneGeneration] = fmt.Sprintf("%d", plane.Generation)
	u.SetAnnotations(annotations)
}

// isNamespaced returns true if the resource kind is typically namespaced
// This is a simplified check - in production, use discovery client
func isNamespaced(u *unstructured.Unstructured) bool {
	// Well-known cluster-scoped resources
	clusterScoped := map[string]bool{
		"Namespace":                     true,
		"Node":                          true,
		"PersistentVolume":              true,
		"ClusterRole":                   true,
		"ClusterRoleBinding":            true,
		"CustomResourceDefinition":      true,
		"APIService":                    true,
		"MutatingWebhookConfiguration":  true,
		"ValidatingWebhookConfiguration": true,
		"PriorityClass":                 true,
		"StorageClass":                  true,
		"IngressClass":                  true,
		"RuntimeClass":                  true,
		"CSIDriver":                     true,
		"CSINode":                       true,
		"VolumeAttachment":              true,
		// OAM cluster-scoped resources
		"ComponentDefinition":      true,
		"TraitDefinition":          true,
		"PolicyDefinition":         true,
		"WorkflowStepDefinition":   true,
		"ClusterPlane":             true,
		"ClusterPlaneRevision":     true,
		"ClusterBlueprint":         true,
		"ClusterBlueprintRevision": true,
	}

	return !clusterScoped[u.GetKind()]
}

// ComponentRenderFunc is a function type for custom component rendering
type ComponentRenderFunc func(ctx context.Context, plane *v1alpha1.ClusterPlane, component *v1alpha1.PlaneComponent) ([]*unstructured.Unstructured, error)

// FuncRenderer wraps a ComponentRenderFunc into a ComponentRenderer
type FuncRenderer struct {
	typeNames []string
	renderFn  ComponentRenderFunc
}

// NewFuncRenderer creates a renderer from a function
func NewFuncRenderer(types []string, fn ComponentRenderFunc) *FuncRenderer {
	return &FuncRenderer{
		typeNames: types,
		renderFn:  fn,
	}
}

// SupportsType checks if this renderer supports the given type
func (fr *FuncRenderer) SupportsType(componentType string) bool {
	for _, t := range fr.typeNames {
		if t == componentType {
			return true
		}
	}
	return false
}

// Render delegates to the wrapped function
func (fr *FuncRenderer) Render(ctx context.Context, plane *v1alpha1.ClusterPlane, component *v1alpha1.PlaneComponent) ([]*unstructured.Unstructured, error) {
	return fr.renderFn(ctx, plane, component)
}

// ParseComponentProperties parses component properties into a typed struct
func ParseComponentProperties(component *v1alpha1.PlaneComponent, into interface{}) error {
	if component.Properties == nil || len(component.Properties.Raw) == 0 {
		return nil
	}
	return json.Unmarshal(component.Properties.Raw, into)
}

// ToRawExtension converts an object to runtime.RawExtension
func ToRawExtension(obj interface{}) (*runtime.RawExtension, error) {
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: raw}, nil
}

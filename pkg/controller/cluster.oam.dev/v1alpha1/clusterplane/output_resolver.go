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
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
)

// OutputResolver handles resolution of PlaneOutputs for ClusterPlanes
type OutputResolver struct {
	client client.Client
}

// NewOutputResolver creates a new OutputResolver
func NewOutputResolver(cli client.Client) *OutputResolver {
	return &OutputResolver{client: cli}
}

// ResolveOutputs resolves all PlaneOutputs for a ClusterPlane
// It extracts values from component resources and populates status.outputs
// Returns true if all outputs are resolved, false otherwise
func (r *OutputResolver) ResolveOutputs(ctx context.Context, plane *v1alpha1.ClusterPlane) (bool, error) {
	if len(plane.Spec.Outputs) == 0 {
		// No outputs to resolve
		plane.Status.Outputs = nil
		plane.Status.ResolvedOutputs = nil
		plane.Status.OutputResolutionSummary = nil
		return true, nil
	}

	now := time.Now().Format(time.RFC3339)
	resolvedOutputs := make([]v1alpha1.ResolvedOutputStatus, 0, len(plane.Spec.Outputs))
	outputValues := make(map[string]string)

	// Build component name to status map for quick lookup
	componentStatuses := buildComponentStatusMap(plane)

	for _, output := range plane.Spec.Outputs {
		resolved := r.resolveOutput(ctx, plane, output, componentStatuses, now)
		resolvedOutputs = append(resolvedOutputs, resolved)

		if resolved.Resolved {
			outputValues[output.Name] = resolved.Value
		}
	}

	plane.Status.Outputs = outputValues
	plane.Status.ResolvedOutputs = resolvedOutputs
	plane.Status.OutputResolutionSummary = calculateOutputSummary(resolvedOutputs)

	return plane.Status.OutputResolutionSummary.AllResolved, nil
}

// resolveOutput resolves a single PlaneOutput
func (r *OutputResolver) resolveOutput(
	ctx context.Context,
	plane *v1alpha1.ClusterPlane,
	output v1alpha1.PlaneOutput,
	componentStatuses map[string]*v1alpha1.ComponentHealthStatus,
	now string,
) v1alpha1.ResolvedOutputStatus {
	status := v1alpha1.ResolvedOutputStatus{
		Name:             output.Name,
		Component:        output.ValueFrom.Component,
		FieldPath:        output.ValueFrom.FieldPath,
		LastResolvedTime: now,
	}

	// Check if the component exists
	compStatus, exists := componentStatuses[output.ValueFrom.Component]
	if !exists {
		status.Error = fmt.Sprintf("component %q not found in plane", output.ValueFrom.Component)
		klog.V(4).InfoS("Output resolution failed: component not found",
			"plane", klog.KRef(plane.Namespace, plane.Name),
			"output", output.Name,
			"component", output.ValueFrom.Component)
		return status
	}

	// Check if component is healthy enough to extract outputs
	if compStatus.Phase != v1alpha1.ComponentPhaseRunning && compStatus.Phase != v1alpha1.ComponentPhaseDegraded {
		status.Error = fmt.Sprintf("component %q is not running (phase: %s)", output.ValueFrom.Component, compStatus.Phase)
		klog.V(4).InfoS("Output resolution skipped: component not running",
			"plane", klog.KRef(plane.Namespace, plane.Name),
			"output", output.Name,
			"component", output.ValueFrom.Component,
			"phase", compStatus.Phase)
		return status
	}

	// Get the component resource and extract value
	value, err := r.extractValueFromComponent(ctx, plane, output)
	if err != nil {
		status.Error = fmt.Sprintf("failed to extract value: %v", err)
		klog.V(4).InfoS("Output resolution failed",
			"plane", klog.KRef(plane.Namespace, plane.Name),
			"output", output.Name,
			"component", output.ValueFrom.Component,
			"fieldPath", output.ValueFrom.FieldPath,
			"error", err)
		return status
	}

	status.Resolved = true
	status.Value = value
	status.Error = ""

	klog.V(4).InfoS("Successfully resolved output",
		"plane", klog.KRef(plane.Namespace, plane.Name),
		"output", output.Name,
		"component", output.ValueFrom.Component,
		"fieldPath", output.ValueFrom.FieldPath)

	return status
}

// extractValueFromComponent extracts a value from a component resource using fieldpath
func (r *OutputResolver) extractValueFromComponent(
	ctx context.Context,
	plane *v1alpha1.ClusterPlane,
	output v1alpha1.PlaneOutput,
) (string, error) {
	// Find the component spec to get resource information
	var componentSpec *v1alpha1.PlaneComponent
	for i := range plane.Spec.Components {
		if plane.Spec.Components[i].Name == output.ValueFrom.Component {
			componentSpec = &plane.Spec.Components[i]
			break
		}
	}

	if componentSpec == nil {
		return "", fmt.Errorf("component %q not found in spec", output.ValueFrom.Component)
	}

	// For now, we try to extract from component details in status
	// In a full implementation, this would look up the actual deployed resources
	// tracked by ResourceTracker and extract from their status

	// Check if value is already available in component details
	compStatus := GetComponentStatus(plane, output.ValueFrom.Component)
	if compStatus != nil && compStatus.Details != nil {
		// Try to get from pre-populated details
		if value, ok := compStatus.Details[output.ValueFrom.FieldPath]; ok {
			return value, nil
		}
	}

	// Try to extract from the ResourceTracker-managed resources
	if plane.Status.ResourceTrackerRef != nil {
		return r.extractFromTrackedResources(ctx, plane, output)
	}

	// If no ResourceTracker, try direct lookup based on component naming convention
	return r.extractFromDirectLookup(ctx, plane, output, componentSpec)
}

// extractFromTrackedResources extracts value from resources tracked by ResourceTracker
func (r *OutputResolver) extractFromTrackedResources(
	ctx context.Context,
	plane *v1alpha1.ClusterPlane,
	output v1alpha1.PlaneOutput,
) (string, error) {
	// In a complete implementation, this would:
	// 1. Load the ResourceTracker
	// 2. Find the managed resource for the component
	// 3. Load that resource
	// 4. Extract the value using fieldpath

	// For now, return an error indicating this needs ResourceTracker integration
	return "", fmt.Errorf("resource tracker extraction not yet implemented for component %q", output.ValueFrom.Component)
}

// extractFromDirectLookup tries to find the component resource directly
func (r *OutputResolver) extractFromDirectLookup(
	ctx context.Context,
	plane *v1alpha1.ClusterPlane,
	output v1alpha1.PlaneOutput,
	componentSpec *v1alpha1.PlaneComponent,
) (string, error) {
	// Build expected resource name based on naming convention
	// Convention: <plane-name>-<component-name>
	resourceName := fmt.Sprintf("%s-%s", plane.Name, output.ValueFrom.Component)

	// Try to find the resource as an unstructured object
	// We start with common Kubernetes resources that outputs typically come from

	// Try Service first (common for endpoint outputs)
	if value, err := r.tryExtractFromResource(ctx, plane.Namespace, resourceName, "v1", "Service", output.ValueFrom.FieldPath); err == nil {
		return value, nil
	}

	// Try ConfigMap (common for configuration outputs)
	if value, err := r.tryExtractFromResource(ctx, plane.Namespace, resourceName, "v1", "ConfigMap", output.ValueFrom.FieldPath); err == nil {
		return value, nil
	}

	// Try Secret (common for credential outputs)
	if value, err := r.tryExtractFromResource(ctx, plane.Namespace, resourceName, "v1", "Secret", output.ValueFrom.FieldPath); err == nil {
		return value, nil
	}

	// Try Deployment
	if value, err := r.tryExtractFromResource(ctx, plane.Namespace, resourceName, "apps/v1", "Deployment", output.ValueFrom.FieldPath); err == nil {
		return value, nil
	}

	return "", fmt.Errorf("could not find resource for component %q to extract field %q", output.ValueFrom.Component, output.ValueFrom.FieldPath)
}

// tryExtractFromResource attempts to extract a value from a specific resource type
func (r *OutputResolver) tryExtractFromResource(
	ctx context.Context,
	namespace, name, apiVersion, kind, fieldPath string,
) (string, error) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(apiVersion)
	obj.SetKind(kind)

	key := client.ObjectKey{Namespace: namespace, Name: name}
	if err := r.client.Get(ctx, key, obj); err != nil {
		return "", err
	}

	return ExtractFieldValue(obj, fieldPath)
}

// ExtractFieldValue extracts a value from an unstructured object using a field path
func ExtractFieldValue(obj *unstructured.Unstructured, path string) (string, error) {
	if obj == nil {
		return "", fmt.Errorf("object is nil")
	}

	paved := fieldpath.Pave(obj.UnstructuredContent())
	value, err := paved.GetValue(path)
	if err != nil {
		return "", fmt.Errorf("failed to get field %q: %w", path, err)
	}

	return formatFieldValue(value), nil
}

// formatFieldValue converts a field value to a string representation
func formatFieldValue(value interface{}) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	case bool:
		return fmt.Sprintf("%t", v)
	case int, int32, int64, float32, float64:
		return fmt.Sprintf("%v", v)
	case []interface{}:
		// For arrays, try to get the first element if it's a simple value
		if len(v) > 0 {
			return formatFieldValue(v[0])
		}
		return ""
	case map[string]interface{}:
		// For maps, return JSON representation
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// buildComponentStatusMap creates a map of component name to status for quick lookup
func buildComponentStatusMap(plane *v1alpha1.ClusterPlane) map[string]*v1alpha1.ComponentHealthStatus {
	statusMap := make(map[string]*v1alpha1.ComponentHealthStatus)
	for i := range plane.Status.ComponentHealth {
		status := &plane.Status.ComponentHealth[i]
		statusMap[status.Name] = status
	}
	return statusMap
}

// calculateOutputSummary calculates the aggregate output resolution status
func calculateOutputSummary(resolved []v1alpha1.ResolvedOutputStatus) *v1alpha1.OutputResolutionSummary {
	if len(resolved) == 0 {
		return nil
	}

	summary := &v1alpha1.OutputResolutionSummary{
		TotalOutputs: len(resolved),
	}

	for _, status := range resolved {
		if status.Resolved {
			summary.ResolvedOutputs++
		} else if status.Error != "" {
			summary.FailedOutputs++
		} else {
			summary.PendingOutputs++
		}
	}

	summary.AllResolved = summary.ResolvedOutputs == summary.TotalOutputs

	return summary
}

// GetResolvedOutputValue returns the resolved value for a given output name
func GetResolvedOutputValue(plane *v1alpha1.ClusterPlane, outputName string) (string, bool) {
	if plane.Status.Outputs == nil {
		return "", false
	}
	value, found := plane.Status.Outputs[outputName]
	return value, found
}

// HasUnresolvedOutputs checks if there are any unresolved outputs
func HasUnresolvedOutputs(plane *v1alpha1.ClusterPlane) bool {
	if plane.Status.OutputResolutionSummary == nil {
		return len(plane.Spec.Outputs) > 0
	}
	return !plane.Status.OutputResolutionSummary.AllResolved
}

// SetOutputValue directly sets an output value in the plane status
// This is useful for components that directly report their outputs
func SetOutputValue(plane *v1alpha1.ClusterPlane, name, value string) {
	if plane.Status.Outputs == nil {
		plane.Status.Outputs = make(map[string]string)
	}
	plane.Status.Outputs[name] = value
}

// SetComponentOutputDetail sets an output value in component details for extraction
// This allows components to pre-populate values that will be extracted as outputs
func SetComponentOutputDetail(plane *v1alpha1.ClusterPlane, componentName, fieldPath, value string) {
	SetComponentDetail(plane, componentName, fieldPath, value)
}

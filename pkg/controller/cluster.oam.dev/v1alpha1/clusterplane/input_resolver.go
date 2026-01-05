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
	"time"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
)

const (
	// ClusterLocal represents the local/hub cluster
	ClusterLocal = "local"
)

// InputResolver handles resolution of CrossClusterInputs for ClusterPlanes
type InputResolver struct {
	client client.Client
}

// NewInputResolver creates a new InputResolver
func NewInputResolver(cli client.Client) *InputResolver {
	return &InputResolver{client: cli}
}

// ResolveInputs resolves all CrossClusterInputs for a ClusterPlane
// Returns true if all required inputs are resolved, false otherwise
func (r *InputResolver) ResolveInputs(ctx context.Context, plane *v1alpha1.ClusterPlane) (bool, error) {
	if len(plane.Spec.CrossClusterInputs) == 0 {
		// No inputs to resolve
		plane.Status.ResolvedInputs = nil
		plane.Status.InputResolutionSummary = nil
		return true, nil
	}

	now := time.Now().Format(time.RFC3339)
	resolvedInputs := make([]v1alpha1.ResolvedInputStatus, 0, len(plane.Spec.CrossClusterInputs))

	for _, input := range plane.Spec.CrossClusterInputs {
		resolved := r.resolveInput(ctx, plane, input, now)
		resolvedInputs = append(resolvedInputs, resolved)
	}

	plane.Status.ResolvedInputs = resolvedInputs
	plane.Status.InputResolutionSummary = calculateInputSummary(plane.Spec.CrossClusterInputs, resolvedInputs)

	return plane.Status.InputResolutionSummary.AllResolved, nil
}

// resolveInput resolves a single CrossClusterInput
func (r *InputResolver) resolveInput(ctx context.Context, plane *v1alpha1.ClusterPlane, input v1alpha1.CrossClusterInput, now string) v1alpha1.ResolvedInputStatus {
	status := v1alpha1.ResolvedInputStatus{
		Name:             input.Name,
		FromCluster:      input.FromCluster,
		FromPlane:        input.FromPlane,
		FromNamespace:    input.FromNamespace,
		Output:           input.Output,
		LastResolvedTime: now,
	}

	// Determine source namespace (default to same namespace as current plane)
	sourceNamespace := input.FromNamespace
	if sourceNamespace == "" {
		sourceNamespace = plane.Namespace
	}

	// Resolve the source plane
	sourcePlane, err := r.getSourcePlane(ctx, input.FromCluster, sourceNamespace, input.FromPlane)
	if err != nil {
		klog.V(4).InfoS("Failed to get source plane",
			"plane", klog.KRef(plane.Namespace, plane.Name),
			"sourcePlane", input.FromPlane,
			"sourceCluster", input.FromCluster,
			"error", err)

		status.Error = fmt.Sprintf("failed to get source plane: %v", err)
		return r.handleResolutionFailure(input, status)
	}

	// Get the output value from the source plane
	value, found := getOutputValue(sourcePlane, input.Output)
	if !found {
		klog.V(4).InfoS("Output not found in source plane",
			"plane", klog.KRef(plane.Namespace, plane.Name),
			"sourcePlane", input.FromPlane,
			"output", input.Output)

		status.Error = fmt.Sprintf("output %q not found in source plane", input.Output)
		return r.handleResolutionFailure(input, status)
	}

	// Successfully resolved
	status.Resolved = true
	status.Value = value
	status.Error = ""

	klog.V(4).InfoS("Successfully resolved input",
		"plane", klog.KRef(plane.Namespace, plane.Name),
		"input", input.Name,
		"sourcePlane", input.FromPlane,
		"output", input.Output)

	return status
}

// getSourcePlane fetches the source ClusterPlane from the specified cluster
func (r *InputResolver) getSourcePlane(ctx context.Context, clusterName, namespace, planeName string) (*v1alpha1.ClusterPlane, error) {
	var sourcePlane v1alpha1.ClusterPlane
	key := client.ObjectKey{Namespace: namespace, Name: planeName}

	// Check if this is the local cluster
	if isLocalCluster(clusterName) {
		if err := r.client.Get(ctx, key, &sourcePlane); err != nil {
			return nil, fmt.Errorf("failed to get plane from local cluster: %w", err)
		}
		return &sourcePlane, nil
	}

	// For remote clusters, use multicluster context
	clusterCtx := pkgmulticluster.WithCluster(ctx, clusterName)
	if err := r.client.Get(clusterCtx, key, &sourcePlane); err != nil {
		return nil, fmt.Errorf("failed to get plane from cluster %s: %w", clusterName, err)
	}

	return &sourcePlane, nil
}

// handleResolutionFailure handles a failed input resolution, applying fallback if available
func (r *InputResolver) handleResolutionFailure(input v1alpha1.CrossClusterInput, status v1alpha1.ResolvedInputStatus) v1alpha1.ResolvedInputStatus {
	// If not required and has fallback, use fallback
	if !input.Required && input.FallbackValue != nil {
		fallbackValue, err := extractFallbackValue(input.FallbackValue)
		if err == nil {
			status.Resolved = true
			status.Value = fallbackValue
			status.UsedFallback = true
			status.Error = ""
			return status
		}
		status.Error = fmt.Sprintf("%s; fallback extraction failed: %v", status.Error, err)
	}

	status.Resolved = false
	return status
}

// getOutputValue retrieves an output value from a ClusterPlane's status
func getOutputValue(plane *v1alpha1.ClusterPlane, outputName string) (string, bool) {
	if plane.Status.Outputs == nil {
		return "", false
	}
	value, found := plane.Status.Outputs[outputName]
	return value, found
}

// extractFallbackValue extracts the string value from a RawExtension fallback
func extractFallbackValue(raw *runtime.RawExtension) (string, error) {
	if raw == nil || raw.Raw == nil {
		return "", fmt.Errorf("fallback value is nil")
	}

	// Try to unmarshal as a simple string
	var strValue string
	if err := json.Unmarshal(raw.Raw, &strValue); err == nil {
		return strValue, nil
	}

	// Return raw JSON as string
	return string(raw.Raw), nil
}

// isLocalCluster checks if the cluster name refers to the local cluster
func isLocalCluster(clusterName string) bool {
	return clusterName == "" || clusterName == ClusterLocal || clusterName == multicluster.ClusterLocalName
}

// calculateInputSummary calculates the aggregate input resolution status
func calculateInputSummary(inputs []v1alpha1.CrossClusterInput, resolved []v1alpha1.ResolvedInputStatus) *v1alpha1.InputResolutionSummary {
	if len(inputs) == 0 {
		return nil
	}

	summary := &v1alpha1.InputResolutionSummary{
		TotalInputs: len(inputs),
	}

	// Build a map of required inputs for quick lookup
	requiredInputs := make(map[string]bool)
	for _, input := range inputs {
		requiredInputs[input.Name] = input.Required
	}

	allRequiredResolved := true
	for _, status := range resolved {
		if status.Resolved {
			summary.ResolvedInputs++
		} else {
			summary.FailedInputs++
			// Check if this failed input was required
			if required, exists := requiredInputs[status.Name]; exists && required {
				allRequiredResolved = false
			}
		}
	}

	summary.PendingInputs = summary.TotalInputs - summary.ResolvedInputs - summary.FailedInputs
	summary.AllResolved = allRequiredResolved && summary.FailedInputs == 0

	return summary
}

// GetResolvedInputValue returns the resolved value for a given input name
func GetResolvedInputValue(plane *v1alpha1.ClusterPlane, inputName string) (string, bool) {
	for _, resolved := range plane.Status.ResolvedInputs {
		if resolved.Name == inputName && resolved.Resolved {
			return resolved.Value, true
		}
	}
	return "", false
}

// GetResolvedInputsMap returns all resolved inputs as a map for templating
func GetResolvedInputsMap(plane *v1alpha1.ClusterPlane) map[string]string {
	result := make(map[string]string)
	for _, resolved := range plane.Status.ResolvedInputs {
		if resolved.Resolved {
			result[resolved.Name] = resolved.Value
		}
	}
	return result
}

// HasUnresolvedRequiredInputs checks if there are any unresolved required inputs
func HasUnresolvedRequiredInputs(plane *v1alpha1.ClusterPlane) bool {
	if plane.Status.InputResolutionSummary == nil {
		return len(plane.Spec.CrossClusterInputs) > 0
	}
	return !plane.Status.InputResolutionSummary.AllResolved
}

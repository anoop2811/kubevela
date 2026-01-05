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
	"regexp"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
)

const (
	// AnnotationPublishVersion is the annotation key for publishing a plane revision
	AnnotationPublishVersion = "plane.oam.dev/publishVersion"

	// MaxComponentNameLength is the maximum length for component names
	MaxComponentNameLength = 63

	// MaxDescriptionLength is the maximum length for description
	MaxDescriptionLength = 1024
)

var (
	// dns1123LabelRegexp matches valid DNS-1123 label names
	dns1123LabelRegexp = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

	// semverRegexp matches semantic version strings (simplified)
	semverRegexp = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+)*(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`)
)

// ValidateCreate validates a ClusterPlane on creation
func (h *ValidatingHandler) ValidateCreate(ctx context.Context, plane *v1alpha1.ClusterPlane) field.ErrorList {
	var errs field.ErrorList

	errs = append(errs, ValidateClusterPlaneSpec(plane)...)
	errs = append(errs, ValidateComponentNames(plane)...)
	errs = append(errs, ValidateComponentTypes(plane)...)
	errs = append(errs, ValidateTraits(plane)...)
	errs = append(errs, ValidatePolicies(plane)...)
	errs = append(errs, ValidateOutputs(plane)...)
	errs = append(errs, ValidateAnnotations(plane)...)

	return errs
}

// ValidateUpdate validates a ClusterPlane on update
func (h *ValidatingHandler) ValidateUpdate(ctx context.Context, newPlane, oldPlane *v1alpha1.ClusterPlane) field.ErrorList {
	var errs field.ErrorList

	// First, run all create validations on the new plane
	errs = append(errs, h.ValidateCreate(ctx, newPlane)...)

	// Additional update-specific validations
	errs = append(errs, ValidateImmutableFields(newPlane, oldPlane)...)

	return errs
}

// ValidateClusterPlaneSpec validates the overall spec structure
func ValidateClusterPlaneSpec(plane *v1alpha1.ClusterPlane) field.ErrorList {
	var errs field.ErrorList
	specPath := field.NewPath("spec")

	// Description length check
	if len(plane.Spec.Description) > MaxDescriptionLength {
		errs = append(errs, field.TooLong(
			specPath.Child("description"),
			plane.Spec.Description,
			MaxDescriptionLength))
	}

	// At least one component is recommended (warning-level, but we'll allow empty for drafts)
	// No error here - empty components are allowed for draft planes

	return errs
}

// ValidateComponentNames validates that component names are unique and valid
func ValidateComponentNames(plane *v1alpha1.ClusterPlane) field.ErrorList {
	var errs field.ErrorList
	componentsPath := field.NewPath("spec", "components")

	componentNames := make(map[string]int)
	for i, comp := range plane.Spec.Components {
		compPath := componentsPath.Index(i)

		// Check name is not empty
		if comp.Name == "" {
			errs = append(errs, field.Required(
				compPath.Child("name"),
				"component name is required"))
			continue
		}

		// Check name length
		if len(comp.Name) > MaxComponentNameLength {
			errs = append(errs, field.TooLong(
				compPath.Child("name"),
				comp.Name,
				MaxComponentNameLength))
		}

		// Check name format (DNS-1123 label)
		if !dns1123LabelRegexp.MatchString(comp.Name) {
			errs = append(errs, field.Invalid(
				compPath.Child("name"),
				comp.Name,
				"must be a valid DNS-1123 label (lowercase alphanumeric with hyphens, not starting/ending with hyphen)"))
		}

		// Check for duplicate names
		if prevIdx, exists := componentNames[comp.Name]; exists {
			errs = append(errs, field.Duplicate(
				compPath.Child("name"),
				fmt.Sprintf("component name %q is duplicated (first occurrence at index %d)", comp.Name, prevIdx)))
		}
		componentNames[comp.Name] = i
	}

	return errs
}

// ValidateComponentTypes validates that component types are specified
func ValidateComponentTypes(plane *v1alpha1.ClusterPlane) field.ErrorList {
	var errs field.ErrorList
	componentsPath := field.NewPath("spec", "components")

	for i, comp := range plane.Spec.Components {
		compPath := componentsPath.Index(i)

		// Check type is not empty
		if comp.Type == "" {
			errs = append(errs, field.Required(
				compPath.Child("type"),
				"component type is required"))
		}
	}

	return errs
}

// ValidateTraits validates traits configuration
func ValidateTraits(plane *v1alpha1.ClusterPlane) field.ErrorList {
	var errs field.ErrorList
	traitsPath := field.NewPath("spec", "traits")

	for i, trait := range plane.Spec.Traits {
		traitPath := traitsPath.Index(i)

		// Check type is not empty
		if trait.Type == "" {
			errs = append(errs, field.Required(
				traitPath.Child("type"),
				"trait type is required"))
		}
	}

	return errs
}

// ValidatePolicies validates policies configuration
func ValidatePolicies(plane *v1alpha1.ClusterPlane) field.ErrorList {
	var errs field.ErrorList
	policiesPath := field.NewPath("spec", "policies")

	policyNames := make(map[string]int)
	for i, policy := range plane.Spec.Policies {
		policyPath := policiesPath.Index(i)

		// Check type is not empty
		if policy.Type == "" {
			errs = append(errs, field.Required(
				policyPath.Child("type"),
				"policy type is required"))
		}

		// Check for duplicate names (if name is set)
		if policy.Name != "" {
			if prevIdx, exists := policyNames[policy.Name]; exists {
				errs = append(errs, field.Duplicate(
					policyPath.Child("name"),
					fmt.Sprintf("policy name %q is duplicated (first occurrence at index %d)", policy.Name, prevIdx)))
			}
			policyNames[policy.Name] = i
		}
	}

	return errs
}

// ValidateOutputs validates outputs configuration
func ValidateOutputs(plane *v1alpha1.ClusterPlane) field.ErrorList {
	var errs field.ErrorList
	outputsPath := field.NewPath("spec", "outputs")

	outputNames := make(map[string]int)
	for i, output := range plane.Spec.Outputs {
		outputPath := outputsPath.Index(i)

		// Check name is not empty
		if output.Name == "" {
			errs = append(errs, field.Required(
				outputPath.Child("name"),
				"output name is required"))
			continue
		}

		// Check for duplicate names
		if prevIdx, exists := outputNames[output.Name]; exists {
			errs = append(errs, field.Duplicate(
				outputPath.Child("name"),
				fmt.Sprintf("output name %q is duplicated (first occurrence at index %d)", output.Name, prevIdx)))
		}
		outputNames[output.Name] = i

		// Check valueFrom is properly configured
		if output.ValueFrom.Component == "" {
			errs = append(errs, field.Required(
				outputPath.Child("valueFrom", "component"),
				"output valueFrom.component is required"))
		}

		if output.ValueFrom.FieldPath == "" {
			errs = append(errs, field.Required(
				outputPath.Child("valueFrom", "fieldPath"),
				"output valueFrom.fieldPath is required"))
		}
	}

	return errs
}

// ValidateAnnotations validates ClusterPlane annotations
func ValidateAnnotations(plane *v1alpha1.ClusterPlane) field.ErrorList {
	var errs field.ErrorList
	annotationsPath := field.NewPath("metadata", "annotations")

	// Validate publishVersion format if present
	if publishVersion, exists := plane.Annotations[AnnotationPublishVersion]; exists {
		if publishVersion == "" {
			errs = append(errs, field.Invalid(
				annotationsPath.Key(AnnotationPublishVersion),
				publishVersion,
				"publishVersion annotation value cannot be empty if set"))
		} else if !semverRegexp.MatchString(publishVersion) {
			errs = append(errs, field.Invalid(
				annotationsPath.Key(AnnotationPublishVersion),
				publishVersion,
				"publishVersion should follow semantic versioning format (e.g., v1.0.0, 1.0.0)"))
		}
	}

	return errs
}

// ValidateImmutableFields validates fields that cannot be changed after creation
func ValidateImmutableFields(newPlane, oldPlane *v1alpha1.ClusterPlane) field.ErrorList {
	var errs field.ErrorList

	// Once a plane is published (has a revision), certain changes may be restricted
	// For now, we allow all changes but could add immutability rules here

	// Example: Prevent changing owner team after publication
	if oldPlane.Status.CurrentRevision != nil && oldPlane.Status.CurrentRevision.Name != "" {
		if oldPlane.Spec.Owner != nil && newPlane.Spec.Owner != nil {
			if oldPlane.Spec.Owner.Team != newPlane.Spec.Owner.Team {
				// This is a warning-level validation, not blocking
				// In production, you might want to make this an error
			}
		}
	}

	return errs
}

// ValidateComponentReferences validates that component references in traits/policies exist
func ValidateComponentReferences(plane *v1alpha1.ClusterPlane) field.ErrorList {
	var errs field.ErrorList

	// Build a set of component names
	componentNames := make(map[string]bool)
	for _, comp := range plane.Spec.Components {
		componentNames[comp.Name] = true
	}

	// Validate output references
	outputsPath := field.NewPath("spec", "outputs")
	for i, output := range plane.Spec.Outputs {
		if output.ValueFrom.Component != "" && !componentNames[output.ValueFrom.Component] {
			errs = append(errs, field.NotFound(
				outputsPath.Index(i).Child("valueFrom", "component"),
				output.ValueFrom.Component))
		}
	}

	return errs
}

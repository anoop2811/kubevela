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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
)

var _ = Describe("Input Resolution Functions", func() {

	Describe("getOutputValue", func() {
		It("should return empty string and false when outputs is nil", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					Outputs: nil,
				},
			}

			value, found := getOutputValue(plane, "endpoint")

			Expect(found).Should(BeFalse())
			Expect(value).Should(BeEmpty())
		})

		It("should return empty string and false when output not found", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					Outputs: map[string]string{
						"other-output": "some-value",
					},
				},
			}

			value, found := getOutputValue(plane, "endpoint")

			Expect(found).Should(BeFalse())
			Expect(value).Should(BeEmpty())
		})

		It("should return value and true when output exists", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					Outputs: map[string]string{
						"endpoint": "https://api.example.com",
					},
				},
			}

			value, found := getOutputValue(plane, "endpoint")

			Expect(found).Should(BeTrue())
			Expect(value).Should(Equal("https://api.example.com"))
		})
	})

	Describe("extractFallbackValue", func() {
		It("should return error when raw is nil", func() {
			_, err := extractFallbackValue(nil)
			Expect(err).Should(HaveOccurred())
		})

		It("should return error when raw.Raw is nil", func() {
			raw := &runtime.RawExtension{Raw: nil}
			_, err := extractFallbackValue(raw)
			Expect(err).Should(HaveOccurred())
		})

		It("should extract string value from JSON", func() {
			raw := &runtime.RawExtension{
				Raw: []byte(`"default-value"`),
			}

			value, err := extractFallbackValue(raw)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(value).Should(Equal("default-value"))
		})

		It("should return raw JSON when not a simple string", func() {
			rawJSON := `{"key":"value","nested":true}`
			raw := &runtime.RawExtension{
				Raw: []byte(rawJSON),
			}

			value, err := extractFallbackValue(raw)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(value).Should(Equal(rawJSON))
		})
	})

	Describe("isLocalCluster", func() {
		It("should return true for empty string", func() {
			Expect(isLocalCluster("")).Should(BeTrue())
		})

		It("should return true for 'local'", func() {
			Expect(isLocalCluster("local")).Should(BeTrue())
		})

		It("should return false for remote cluster names", func() {
			Expect(isLocalCluster("cluster-west")).Should(BeFalse())
			Expect(isLocalCluster("production")).Should(BeFalse())
		})
	})

	Describe("calculateInputSummary", func() {
		It("should return nil for empty inputs", func() {
			inputs := []v1alpha1.CrossClusterInput{}
			resolved := []v1alpha1.ResolvedInputStatus{}

			summary := calculateInputSummary(inputs, resolved)

			Expect(summary).Should(BeNil())
		})

		It("should calculate correct summary for all resolved", func() {
			inputs := []v1alpha1.CrossClusterInput{
				{Name: "input-1", Required: true},
				{Name: "input-2", Required: false},
			}
			resolved := []v1alpha1.ResolvedInputStatus{
				{Name: "input-1", Resolved: true},
				{Name: "input-2", Resolved: true},
			}

			summary := calculateInputSummary(inputs, resolved)

			Expect(summary).ShouldNot(BeNil())
			Expect(summary.TotalInputs).Should(Equal(2))
			Expect(summary.ResolvedInputs).Should(Equal(2))
			Expect(summary.FailedInputs).Should(Equal(0))
			Expect(summary.PendingInputs).Should(Equal(0))
			Expect(summary.AllResolved).Should(BeTrue())
		})

		It("should report AllResolved=false when required input fails", func() {
			inputs := []v1alpha1.CrossClusterInput{
				{Name: "input-1", Required: true},
				{Name: "input-2", Required: false},
			}
			resolved := []v1alpha1.ResolvedInputStatus{
				{Name: "input-1", Resolved: false},
				{Name: "input-2", Resolved: true},
			}

			summary := calculateInputSummary(inputs, resolved)

			Expect(summary.AllResolved).Should(BeFalse())
			Expect(summary.FailedInputs).Should(Equal(1))
			Expect(summary.ResolvedInputs).Should(Equal(1))
		})

		It("should report AllResolved=true when only optional inputs fail", func() {
			inputs := []v1alpha1.CrossClusterInput{
				{Name: "input-1", Required: true},
				{Name: "input-2", Required: false},
			}
			resolved := []v1alpha1.ResolvedInputStatus{
				{Name: "input-1", Resolved: true},
				{Name: "input-2", Resolved: false},
			}

			summary := calculateInputSummary(inputs, resolved)

			// AllResolved should be false because there's still a failed input
			// even though it's optional
			Expect(summary.AllResolved).Should(BeFalse())
			Expect(summary.FailedInputs).Should(Equal(1))
		})
	})

	Describe("GetResolvedInputValue", func() {
		It("should return empty and false when no resolved inputs", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ResolvedInputs: nil,
				},
			}

			value, found := GetResolvedInputValue(plane, "some-input")

			Expect(found).Should(BeFalse())
			Expect(value).Should(BeEmpty())
		})

		It("should return empty and false when input not found", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ResolvedInputs: []v1alpha1.ResolvedInputStatus{
						{Name: "other-input", Resolved: true, Value: "test"},
					},
				},
			}

			value, found := GetResolvedInputValue(plane, "missing-input")

			Expect(found).Should(BeFalse())
			Expect(value).Should(BeEmpty())
		})

		It("should return empty and false when input exists but not resolved", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ResolvedInputs: []v1alpha1.ResolvedInputStatus{
						{Name: "my-input", Resolved: false, Value: "", Error: "source not found"},
					},
				},
			}

			value, found := GetResolvedInputValue(plane, "my-input")

			Expect(found).Should(BeFalse())
			Expect(value).Should(BeEmpty())
		})

		It("should return value and true when input is resolved", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ResolvedInputs: []v1alpha1.ResolvedInputStatus{
						{Name: "api-endpoint", Resolved: true, Value: "https://api.example.com"},
					},
				},
			}

			value, found := GetResolvedInputValue(plane, "api-endpoint")

			Expect(found).Should(BeTrue())
			Expect(value).Should(Equal("https://api.example.com"))
		})
	})

	Describe("GetResolvedInputsMap", func() {
		It("should return empty map when no resolved inputs", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ResolvedInputs: nil,
				},
			}

			result := GetResolvedInputsMap(plane)

			Expect(result).Should(BeEmpty())
		})

		It("should only include resolved inputs", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ResolvedInputs: []v1alpha1.ResolvedInputStatus{
						{Name: "resolved-1", Resolved: true, Value: "value-1"},
						{Name: "failed-1", Resolved: false, Error: "error"},
						{Name: "resolved-2", Resolved: true, Value: "value-2"},
					},
				},
			}

			result := GetResolvedInputsMap(plane)

			Expect(result).Should(HaveLen(2))
			Expect(result["resolved-1"]).Should(Equal("value-1"))
			Expect(result["resolved-2"]).Should(Equal("value-2"))
			Expect(result).ShouldNot(HaveKey("failed-1"))
		})
	})

	Describe("HasUnresolvedRequiredInputs", func() {
		It("should return true when summary is nil but inputs exist", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{Name: "input-1", Required: true},
					},
				},
				Status: v1alpha1.ClusterPlaneStatus{
					InputResolutionSummary: nil,
				},
			}

			Expect(HasUnresolvedRequiredInputs(plane)).Should(BeTrue())
		})

		It("should return false when summary is nil and no inputs", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: nil,
				},
				Status: v1alpha1.ClusterPlaneStatus{
					InputResolutionSummary: nil,
				},
			}

			Expect(HasUnresolvedRequiredInputs(plane)).Should(BeFalse())
		})

		It("should return false when all resolved", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{Name: "input-1", Required: true},
					},
				},
				Status: v1alpha1.ClusterPlaneStatus{
					InputResolutionSummary: &v1alpha1.InputResolutionSummary{
						AllResolved: true,
					},
				},
			}

			Expect(HasUnresolvedRequiredInputs(plane)).Should(BeFalse())
		})

		It("should return true when not all resolved", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{Name: "input-1", Required: true},
					},
				},
				Status: v1alpha1.ClusterPlaneStatus{
					InputResolutionSummary: &v1alpha1.InputResolutionSummary{
						AllResolved: false,
					},
				},
			}

			Expect(HasUnresolvedRequiredInputs(plane)).Should(BeTrue())
		})
	})
})

var _ = Describe("InputResolver", func() {

	Describe("handleResolutionFailure", func() {
		var resolver *InputResolver

		BeforeEach(func() {
			resolver = &InputResolver{}
		})

		It("should apply fallback for non-required input with fallback value", func() {
			fallbackJSON, _ := json.Marshal("default-endpoint")
			input := v1alpha1.CrossClusterInput{
				Name:     "api-endpoint",
				Required: false,
				FallbackValue: &runtime.RawExtension{
					Raw: fallbackJSON,
				},
			}
			status := v1alpha1.ResolvedInputStatus{
				Name:  "api-endpoint",
				Error: "source plane not found",
			}

			result := resolver.handleResolutionFailure(input, status)

			Expect(result.Resolved).Should(BeTrue())
			Expect(result.Value).Should(Equal("default-endpoint"))
			Expect(result.UsedFallback).Should(BeTrue())
			Expect(result.Error).Should(BeEmpty())
		})

		It("should not apply fallback for required input", func() {
			fallbackJSON, _ := json.Marshal("default-endpoint")
			input := v1alpha1.CrossClusterInput{
				Name:     "api-endpoint",
				Required: true,
				FallbackValue: &runtime.RawExtension{
					Raw: fallbackJSON,
				},
			}
			status := v1alpha1.ResolvedInputStatus{
				Name:  "api-endpoint",
				Error: "source plane not found",
			}

			result := resolver.handleResolutionFailure(input, status)

			Expect(result.Resolved).Should(BeFalse())
			Expect(result.UsedFallback).Should(BeFalse())
			Expect(result.Error).Should(Equal("source plane not found"))
		})

		It("should not apply fallback when no fallback value provided", func() {
			input := v1alpha1.CrossClusterInput{
				Name:          "api-endpoint",
				Required:      false,
				FallbackValue: nil,
			}
			status := v1alpha1.ResolvedInputStatus{
				Name:  "api-endpoint",
				Error: "source plane not found",
			}

			result := resolver.handleResolutionFailure(input, status)

			Expect(result.Resolved).Should(BeFalse())
			Expect(result.UsedFallback).Should(BeFalse())
		})

		It("should append error when fallback extraction fails", func() {
			input := v1alpha1.CrossClusterInput{
				Name:     "api-endpoint",
				Required: false,
				FallbackValue: &runtime.RawExtension{
					Raw: nil, // Will cause extraction to fail
				},
			}
			status := v1alpha1.ResolvedInputStatus{
				Name:  "api-endpoint",
				Error: "source plane not found",
			}

			result := resolver.handleResolutionFailure(input, status)

			Expect(result.Resolved).Should(BeFalse())
			Expect(result.Error).Should(ContainSubstring("source plane not found"))
			Expect(result.Error).Should(ContainSubstring("fallback extraction failed"))
		})
	})
})

var _ = Describe("ResolvedInputStatus", func() {
	It("should properly track resolution metadata", func() {
		status := v1alpha1.ResolvedInputStatus{
			Name:             "cluster-endpoint",
			FromCluster:      "production-west",
			FromPlane:        "networking-plane",
			FromNamespace:    "infra",
			Output:           "loadbalancer-ip",
			Resolved:         true,
			Value:            "10.0.0.50",
			UsedFallback:     false,
			Error:            "",
			LastResolvedTime: "2025-01-01T00:00:00Z",
		}

		Expect(status.Name).Should(Equal("cluster-endpoint"))
		Expect(status.FromCluster).Should(Equal("production-west"))
		Expect(status.FromPlane).Should(Equal("networking-plane"))
		Expect(status.FromNamespace).Should(Equal("infra"))
		Expect(status.Output).Should(Equal("loadbalancer-ip"))
		Expect(status.Resolved).Should(BeTrue())
		Expect(status.Value).Should(Equal("10.0.0.50"))
		Expect(status.UsedFallback).Should(BeFalse())
		Expect(status.Error).Should(BeEmpty())
	})
})

var _ = Describe("InputResolutionSummary", func() {
	It("should correctly represent aggregate state", func() {
		summary := &v1alpha1.InputResolutionSummary{
			TotalInputs:    5,
			ResolvedInputs: 3,
			FailedInputs:   1,
			PendingInputs:  1,
			AllResolved:    false,
		}

		Expect(summary.TotalInputs).Should(Equal(5))
		Expect(summary.ResolvedInputs).Should(Equal(3))
		Expect(summary.FailedInputs).Should(Equal(1))
		Expect(summary.PendingInputs).Should(Equal(1))
		Expect(summary.AllResolved).Should(BeFalse())
	})
})

var _ = Describe("CrossClusterInput Integration", func() {
	It("should be properly defined in ClusterPlane spec", func() {
		plane := &v1alpha1.ClusterPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "data-plane",
				Namespace: "production",
			},
			Spec: v1alpha1.ClusterPlaneSpec{
				CrossClusterInputs: []v1alpha1.CrossClusterInput{
					{
						Name:        "network-cidr",
						FromCluster: "management",
						FromPlane:   "network-plane",
						Output:      "pod-cidr",
						Required:    true,
					},
					{
						Name:          "dns-server",
						FromCluster:   "management",
						FromPlane:     "dns-plane",
						FromNamespace: "dns-system",
						Output:        "server-ip",
						Required:      false,
						FallbackValue: &runtime.RawExtension{
							Raw: []byte(`"8.8.8.8"`),
						},
					},
				},
			},
		}

		Expect(plane.Spec.CrossClusterInputs).Should(HaveLen(2))

		// Verify first input
		input1 := plane.Spec.CrossClusterInputs[0]
		Expect(input1.Name).Should(Equal("network-cidr"))
		Expect(input1.FromCluster).Should(Equal("management"))
		Expect(input1.FromPlane).Should(Equal("network-plane"))
		Expect(input1.Required).Should(BeTrue())

		// Verify second input
		input2 := plane.Spec.CrossClusterInputs[1]
		Expect(input2.Name).Should(Equal("dns-server"))
		Expect(input2.FromNamespace).Should(Equal("dns-system"))
		Expect(input2.Required).Should(BeFalse())
		Expect(input2.FallbackValue).ShouldNot(BeNil())
	})
})

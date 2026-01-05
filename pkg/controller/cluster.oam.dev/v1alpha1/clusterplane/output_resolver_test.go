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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
)

var _ = Describe("Output Resolution Functions", func() {

	Describe("GetResolvedOutputValue", func() {
		It("should return empty string and false when outputs is nil", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					Outputs: nil,
				},
			}

			value, found := GetResolvedOutputValue(plane, "endpoint")

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

			value, found := GetResolvedOutputValue(plane, "endpoint")

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

			value, found := GetResolvedOutputValue(plane, "endpoint")

			Expect(found).Should(BeTrue())
			Expect(value).Should(Equal("https://api.example.com"))
		})
	})

	Describe("HasUnresolvedOutputs", func() {
		It("should return true when summary is nil but outputs exist", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Outputs: []v1alpha1.PlaneOutput{
						{Name: "output-1"},
					},
				},
				Status: v1alpha1.ClusterPlaneStatus{
					OutputResolutionSummary: nil,
				},
			}

			Expect(HasUnresolvedOutputs(plane)).Should(BeTrue())
		})

		It("should return false when summary is nil and no outputs", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Outputs: nil,
				},
				Status: v1alpha1.ClusterPlaneStatus{
					OutputResolutionSummary: nil,
				},
			}

			Expect(HasUnresolvedOutputs(plane)).Should(BeFalse())
		})

		It("should return false when all resolved", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Outputs: []v1alpha1.PlaneOutput{
						{Name: "output-1"},
					},
				},
				Status: v1alpha1.ClusterPlaneStatus{
					OutputResolutionSummary: &v1alpha1.OutputResolutionSummary{
						AllResolved: true,
					},
				},
			}

			Expect(HasUnresolvedOutputs(plane)).Should(BeFalse())
		})

		It("should return true when not all resolved", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Outputs: []v1alpha1.PlaneOutput{
						{Name: "output-1"},
					},
				},
				Status: v1alpha1.ClusterPlaneStatus{
					OutputResolutionSummary: &v1alpha1.OutputResolutionSummary{
						AllResolved: false,
					},
				},
			}

			Expect(HasUnresolvedOutputs(plane)).Should(BeTrue())
		})
	})

	Describe("SetOutputValue", func() {
		It("should create outputs map if nil", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					Outputs: nil,
				},
			}

			SetOutputValue(plane, "endpoint", "https://api.example.com")

			Expect(plane.Status.Outputs).ShouldNot(BeNil())
			Expect(plane.Status.Outputs["endpoint"]).Should(Equal("https://api.example.com"))
		})

		It("should add to existing outputs map", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					Outputs: map[string]string{
						"existing": "value",
					},
				},
			}

			SetOutputValue(plane, "new-output", "new-value")

			Expect(plane.Status.Outputs).Should(HaveLen(2))
			Expect(plane.Status.Outputs["existing"]).Should(Equal("value"))
			Expect(plane.Status.Outputs["new-output"]).Should(Equal("new-value"))
		})

		It("should overwrite existing output", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					Outputs: map[string]string{
						"endpoint": "old-value",
					},
				},
			}

			SetOutputValue(plane, "endpoint", "new-value")

			Expect(plane.Status.Outputs["endpoint"]).Should(Equal("new-value"))
		})
	})

	Describe("calculateOutputSummary", func() {
		It("should return nil for empty outputs", func() {
			resolved := []v1alpha1.ResolvedOutputStatus{}

			summary := calculateOutputSummary(resolved)

			Expect(summary).Should(BeNil())
		})

		It("should calculate correct summary for all resolved", func() {
			resolved := []v1alpha1.ResolvedOutputStatus{
				{Name: "output-1", Resolved: true},
				{Name: "output-2", Resolved: true},
			}

			summary := calculateOutputSummary(resolved)

			Expect(summary).ShouldNot(BeNil())
			Expect(summary.TotalOutputs).Should(Equal(2))
			Expect(summary.ResolvedOutputs).Should(Equal(2))
			Expect(summary.FailedOutputs).Should(Equal(0))
			Expect(summary.PendingOutputs).Should(Equal(0))
			Expect(summary.AllResolved).Should(BeTrue())
		})

		It("should report AllResolved=false when output fails", func() {
			resolved := []v1alpha1.ResolvedOutputStatus{
				{Name: "output-1", Resolved: true},
				{Name: "output-2", Resolved: false, Error: "component not found"},
			}

			summary := calculateOutputSummary(resolved)

			Expect(summary.AllResolved).Should(BeFalse())
			Expect(summary.FailedOutputs).Should(Equal(1))
			Expect(summary.ResolvedOutputs).Should(Equal(1))
		})

		It("should track pending outputs without error", func() {
			resolved := []v1alpha1.ResolvedOutputStatus{
				{Name: "output-1", Resolved: true},
				{Name: "output-2", Resolved: false, Error: ""},
			}

			summary := calculateOutputSummary(resolved)

			Expect(summary.PendingOutputs).Should(Equal(1))
			Expect(summary.FailedOutputs).Should(Equal(0))
		})
	})

	Describe("ExtractFieldValue", func() {
		It("should return error when object is nil", func() {
			_, err := ExtractFieldValue(nil, "status.phase")

			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("object is nil"))
		})

		It("should extract simple string value", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"phase": "Running",
					},
				},
			}

			value, err := ExtractFieldValue(obj, "status.phase")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(value).Should(Equal("Running"))
		})

		It("should extract nested value", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "app",
										"image": "nginx:latest",
									},
								},
							},
						},
					},
				},
			}

			value, err := ExtractFieldValue(obj, "spec.template.spec.containers[0].image")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(value).Should(Equal("nginx:latest"))
		})

		It("should extract boolean value", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"ready": true,
					},
				},
			}

			value, err := ExtractFieldValue(obj, "status.ready")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(value).Should(Equal("true"))
		})

		It("should extract numeric value", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"replicas": int64(3),
					},
				},
			}

			value, err := ExtractFieldValue(obj, "spec.replicas")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(value).Should(Equal("3"))
		})

		It("should return error for non-existent path", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"phase": "Running",
					},
				},
			}

			_, err := ExtractFieldValue(obj, "status.nonexistent.path")

			Expect(err).Should(HaveOccurred())
		})

		It("should extract first array element when array is returned", func() {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"addresses": []interface{}{
							"10.0.0.1",
							"10.0.0.2",
						},
					},
				},
			}

			value, err := ExtractFieldValue(obj, "status.addresses")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(value).Should(Equal("10.0.0.1"))
		})
	})

	Describe("formatFieldValue", func() {
		It("should return empty string for nil", func() {
			result := formatFieldValue(nil)
			Expect(result).Should(BeEmpty())
		})

		It("should return string value as-is", func() {
			result := formatFieldValue("hello")
			Expect(result).Should(Equal("hello"))
		})

		It("should format boolean", func() {
			result := formatFieldValue(true)
			Expect(result).Should(Equal("true"))

			result = formatFieldValue(false)
			Expect(result).Should(Equal("false"))
		})

		It("should format integers", func() {
			result := formatFieldValue(42)
			Expect(result).Should(Equal("42"))

			result = formatFieldValue(int64(100))
			Expect(result).Should(Equal("100"))
		})

		It("should format floats", func() {
			result := formatFieldValue(3.14)
			Expect(result).Should(Equal("3.14"))
		})

		It("should return first element of array", func() {
			arr := []interface{}{"first", "second"}
			result := formatFieldValue(arr)
			Expect(result).Should(Equal("first"))
		})

		It("should return empty string for empty array", func() {
			arr := []interface{}{}
			result := formatFieldValue(arr)
			Expect(result).Should(BeEmpty())
		})
	})

	Describe("buildComponentStatusMap", func() {
		It("should return empty map when no component health", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: nil,
				},
			}

			result := buildComponentStatusMap(plane)

			Expect(result).Should(BeEmpty())
		})

		It("should build map with component statuses", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "database", Phase: v1alpha1.ComponentPhaseRunning, Healthy: true},
						{Name: "cache", Phase: v1alpha1.ComponentPhaseDegraded, Healthy: false},
					},
				},
			}

			result := buildComponentStatusMap(plane)

			Expect(result).Should(HaveLen(2))
			Expect(result["database"].Phase).Should(Equal(v1alpha1.ComponentPhaseRunning))
			Expect(result["cache"].Phase).Should(Equal(v1alpha1.ComponentPhaseDegraded))
		})
	})
})

var _ = Describe("ResolvedOutputStatus", func() {
	It("should properly track resolution metadata", func() {
		status := v1alpha1.ResolvedOutputStatus{
			Name:             "api-endpoint",
			Component:        "api-gateway",
			FieldPath:        "status.loadBalancer.ingress[0].ip",
			Resolved:         true,
			Value:            "10.0.0.50",
			Error:            "",
			LastResolvedTime: "2025-01-01T00:00:00Z",
		}

		Expect(status.Name).Should(Equal("api-endpoint"))
		Expect(status.Component).Should(Equal("api-gateway"))
		Expect(status.FieldPath).Should(Equal("status.loadBalancer.ingress[0].ip"))
		Expect(status.Resolved).Should(BeTrue())
		Expect(status.Value).Should(Equal("10.0.0.50"))
		Expect(status.Error).Should(BeEmpty())
	})

	It("should track failed resolution with error", func() {
		status := v1alpha1.ResolvedOutputStatus{
			Name:      "db-connection",
			Component: "database",
			FieldPath: "status.connectionString",
			Resolved:  false,
			Error:     "component not running (phase: Pending)",
		}

		Expect(status.Resolved).Should(BeFalse())
		Expect(status.Error).Should(ContainSubstring("component not running"))
	})
})

var _ = Describe("OutputResolutionSummary", func() {
	It("should correctly represent aggregate state", func() {
		summary := &v1alpha1.OutputResolutionSummary{
			TotalOutputs:    5,
			ResolvedOutputs: 3,
			FailedOutputs:   1,
			PendingOutputs:  1,
			AllResolved:     false,
		}

		Expect(summary.TotalOutputs).Should(Equal(5))
		Expect(summary.ResolvedOutputs).Should(Equal(3))
		Expect(summary.FailedOutputs).Should(Equal(1))
		Expect(summary.PendingOutputs).Should(Equal(1))
		Expect(summary.AllResolved).Should(BeFalse())
	})

	It("should report AllResolved when all outputs are resolved", func() {
		summary := &v1alpha1.OutputResolutionSummary{
			TotalOutputs:    3,
			ResolvedOutputs: 3,
			FailedOutputs:   0,
			PendingOutputs:  0,
			AllResolved:     true,
		}

		Expect(summary.AllResolved).Should(BeTrue())
		Expect(summary.TotalOutputs).Should(Equal(summary.ResolvedOutputs))
	})
})

var _ = Describe("PlaneOutput Integration", func() {
	It("should be properly defined in ClusterPlane spec", func() {
		plane := &v1alpha1.ClusterPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "networking-plane",
				Namespace: "infrastructure",
			},
			Spec: v1alpha1.ClusterPlaneSpec{
				Components: []v1alpha1.PlaneComponent{
					{Name: "load-balancer", Type: "service"},
					{Name: "ingress-controller", Type: "helm"},
				},
				Outputs: []v1alpha1.PlaneOutput{
					{
						Name: "lb-ip",
						ValueFrom: v1alpha1.PlaneOutputValueFrom{
							Component: "load-balancer",
							FieldPath: "status.loadBalancer.ingress[0].ip",
						},
					},
					{
						Name: "ingress-class",
						ValueFrom: v1alpha1.PlaneOutputValueFrom{
							Component: "ingress-controller",
							FieldPath: "spec.ingressClassName",
						},
					},
				},
			},
		}

		Expect(plane.Spec.Outputs).Should(HaveLen(2))

		// Verify first output
		output1 := plane.Spec.Outputs[0]
		Expect(output1.Name).Should(Equal("lb-ip"))
		Expect(output1.ValueFrom.Component).Should(Equal("load-balancer"))
		Expect(output1.ValueFrom.FieldPath).Should(ContainSubstring("loadBalancer"))

		// Verify second output
		output2 := plane.Spec.Outputs[1]
		Expect(output2.Name).Should(Equal("ingress-class"))
		Expect(output2.ValueFrom.Component).Should(Equal("ingress-controller"))
	})

	It("should track resolved outputs in status", func() {
		plane := &v1alpha1.ClusterPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "networking-plane",
				Namespace: "infrastructure",
			},
			Spec: v1alpha1.ClusterPlaneSpec{
				Outputs: []v1alpha1.PlaneOutput{
					{
						Name: "lb-ip",
						ValueFrom: v1alpha1.PlaneOutputValueFrom{
							Component: "load-balancer",
							FieldPath: "status.loadBalancer.ingress[0].ip",
						},
					},
				},
			},
			Status: v1alpha1.ClusterPlaneStatus{
				Outputs: map[string]string{
					"lb-ip": "192.168.1.100",
				},
				ResolvedOutputs: []v1alpha1.ResolvedOutputStatus{
					{
						Name:             "lb-ip",
						Component:        "load-balancer",
						FieldPath:        "status.loadBalancer.ingress[0].ip",
						Resolved:         true,
						Value:            "192.168.1.100",
						LastResolvedTime: "2025-01-01T00:00:00Z",
					},
				},
				OutputResolutionSummary: &v1alpha1.OutputResolutionSummary{
					TotalOutputs:    1,
					ResolvedOutputs: 1,
					AllResolved:     true,
				},
			},
		}

		// Verify outputs are accessible
		value, found := GetResolvedOutputValue(plane, "lb-ip")
		Expect(found).Should(BeTrue())
		Expect(value).Should(Equal("192.168.1.100"))

		// Verify no unresolved outputs
		Expect(HasUnresolvedOutputs(plane)).Should(BeFalse())
	})
})

var _ = Describe("SetComponentOutputDetail", func() {
	It("should set component detail for output extraction", func() {
		plane := &v1alpha1.ClusterPlane{
			Status: v1alpha1.ClusterPlaneStatus{
				ComponentHealth: []v1alpha1.ComponentHealthStatus{
					{Name: "api-gateway"},
				},
			},
		}

		SetComponentOutputDetail(plane, "api-gateway", "status.endpoint", "https://api.example.com")

		// Verify the detail was set
		compStatus := GetComponentStatus(plane, "api-gateway")
		Expect(compStatus).ShouldNot(BeNil())
		Expect(compStatus.Details).ShouldNot(BeNil())
		Expect(compStatus.Details["status.endpoint"]).Should(Equal("https://api.example.com"))
	})
})

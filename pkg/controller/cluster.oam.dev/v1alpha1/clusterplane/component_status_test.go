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

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
)

var _ = Describe("Component Status Functions", func() {

	Describe("InitializeComponentStatuses", func() {
		It("should handle empty components", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{},
				},
			}

			InitializeComponentStatuses(plane)

			Expect(plane.Status.ComponentHealth).Should(BeEmpty())
			Expect(plane.Status.HealthSummary).Should(BeNil())
		})

		It("should initialize new components with Pending status", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "comp-a", Type: "helm"},
						{Name: "comp-b", Type: "kustomize"},
					},
				},
			}

			InitializeComponentStatuses(plane)

			Expect(plane.Status.ComponentHealth).Should(HaveLen(2))

			csA := GetComponentStatus(plane, "comp-a")
			Expect(csA).ShouldNot(BeNil())
			Expect(csA.Phase).Should(Equal(v1alpha1.ComponentPhasePending))
			Expect(csA.Type).Should(Equal("helm"))

			csB := GetComponentStatus(plane, "comp-b")
			Expect(csB).ShouldNot(BeNil())
			Expect(csB.Phase).Should(Equal(v1alpha1.ComponentPhasePending))
			Expect(csB.Type).Should(Equal("kustomize"))
		})

		It("should preserve existing statuses", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "comp-a", Type: "helm"},
						{Name: "comp-b", Type: "kustomize"},
					},
				},
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "comp-a", Phase: v1alpha1.ComponentPhaseRunning, Healthy: true},
					},
				},
			}

			InitializeComponentStatuses(plane)

			Expect(plane.Status.ComponentHealth).Should(HaveLen(2))

			csA := GetComponentStatus(plane, "comp-a")
			Expect(csA).ShouldNot(BeNil())
			Expect(csA.Phase).Should(Equal(v1alpha1.ComponentPhaseRunning))
			Expect(csA.Healthy).Should(BeTrue())

			csB := GetComponentStatus(plane, "comp-b")
			Expect(csB).ShouldNot(BeNil())
			Expect(csB.Phase).Should(Equal(v1alpha1.ComponentPhasePending))
		})

		It("should clean up removed components", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "comp-a", Type: "helm"},
					},
				},
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "comp-a", Phase: v1alpha1.ComponentPhaseRunning, Healthy: true},
						{Name: "comp-b", Phase: v1alpha1.ComponentPhaseRunning, Healthy: true},
					},
				},
			}

			InitializeComponentStatuses(plane)

			Expect(plane.Status.ComponentHealth).Should(HaveLen(1))
			Expect(GetComponentStatus(plane, "comp-a")).ShouldNot(BeNil())
			Expect(GetComponentStatus(plane, "comp-b")).Should(BeNil())
		})
	})

	Describe("UpdateComponentStatus", func() {
		It("should update existing component", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{
							Name:    "comp-a",
							Phase:   v1alpha1.ComponentPhasePending,
							Healthy: false,
							Reason:  "Pending",
						},
					},
				},
			}

			changed := UpdateComponentStatus(plane, "comp-a",
				v1alpha1.ComponentPhaseRunning, true, "Running", "Healthy")

			Expect(changed).Should(BeTrue())
			cs := GetComponentStatus(plane, "comp-a")
			Expect(cs.Phase).Should(Equal(v1alpha1.ComponentPhaseRunning))
			Expect(cs.Healthy).Should(BeTrue())
			Expect(cs.Reason).Should(Equal("Healthy"))
		})

		It("should return false when values unchanged", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{
							Name:    "comp-a",
							Phase:   v1alpha1.ComponentPhasePending,
							Healthy: false,
							Reason:  "Pending",
						},
					},
				},
			}

			changed := UpdateComponentStatus(plane, "comp-a",
				v1alpha1.ComponentPhasePending, false, "Waiting", "Pending")

			Expect(changed).Should(BeFalse())
		})

		It("should return false for non-existent component", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "comp-a"},
					},
				},
			}

			changed := UpdateComponentStatus(plane, "comp-nonexistent",
				v1alpha1.ComponentPhaseRunning, true, "Test", "Test")

			Expect(changed).Should(BeFalse())
		})
	})

	Describe("CalculateHealthSummary", func() {
		It("should return nil for empty components", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{},
				},
			}

			summary := CalculateHealthSummary(plane)

			Expect(summary).Should(BeNil())
		})

		It("should report all healthy", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "a", Phase: v1alpha1.ComponentPhaseRunning, Healthy: true},
						{Name: "b", Phase: v1alpha1.ComponentPhaseRunning, Healthy: true},
					},
				},
			}

			summary := CalculateHealthSummary(plane)

			Expect(summary).ShouldNot(BeNil())
			Expect(summary.TotalComponents).Should(Equal(2))
			Expect(summary.HealthyComponents).Should(Equal(2))
			Expect(summary.DegradedComponents).Should(Equal(0))
			Expect(summary.FailedComponents).Should(Equal(0))
			Expect(summary.PendingComponents).Should(Equal(0))
			Expect(summary.OverallHealthy).Should(BeTrue())
		})

		It("should report mixed states correctly", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "a", Phase: v1alpha1.ComponentPhaseRunning, Healthy: true},
						{Name: "b", Phase: v1alpha1.ComponentPhaseDegraded, Healthy: false},
						{Name: "c", Phase: v1alpha1.ComponentPhaseFailed, Healthy: false},
						{Name: "d", Phase: v1alpha1.ComponentPhasePending, Healthy: false},
					},
				},
			}

			summary := CalculateHealthSummary(plane)

			Expect(summary.TotalComponents).Should(Equal(4))
			Expect(summary.HealthyComponents).Should(Equal(1))
			Expect(summary.DegradedComponents).Should(Equal(1))
			Expect(summary.FailedComponents).Should(Equal(1))
			Expect(summary.PendingComponents).Should(Equal(1))
			Expect(summary.OverallHealthy).Should(BeFalse())
		})

		It("should count Running but not Healthy as degraded", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "a", Phase: v1alpha1.ComponentPhaseRunning, Healthy: false},
					},
				},
			}

			summary := CalculateHealthSummary(plane)

			Expect(summary.DegradedComponents).Should(Equal(1))
			Expect(summary.HealthyComponents).Should(Equal(0))
		})

		It("should count Deploying as pending", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "a", Phase: v1alpha1.ComponentPhaseDeploying, Healthy: false},
						{Name: "b", Phase: v1alpha1.ComponentPhasePending, Healthy: false},
					},
				},
			}

			summary := CalculateHealthSummary(plane)

			Expect(summary.PendingComponents).Should(Equal(2))
		})
	})

	Describe("DeterminePhaseFromHealth", func() {
		It("should keep current phase when summary is nil", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					Phase:         v1alpha1.PlanePhaseRunning,
					HealthSummary: nil,
				},
			}

			phase := DeterminePhaseFromHealth(plane)

			Expect(phase).Should(Equal(v1alpha1.PlanePhaseRunning))
		})

		It("should return Draft when summary is nil and phase is empty", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					Phase:         "",
					HealthSummary: nil,
				},
			}

			phase := DeterminePhaseFromHealth(plane)

			Expect(phase).Should(Equal(v1alpha1.PlanePhaseDraft))
		})

		It("should return Failed when any component failed", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					HealthSummary: &v1alpha1.PlaneHealthSummary{
						TotalComponents:  3,
						FailedComponents: 1,
					},
				},
			}

			phase := DeterminePhaseFromHealth(plane)

			Expect(phase).Should(Equal(v1alpha1.PlanePhaseFailed))
		})

		It("should return Degraded when any component degraded", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					HealthSummary: &v1alpha1.PlaneHealthSummary{
						TotalComponents:    3,
						DegradedComponents: 1,
					},
				},
			}

			phase := DeterminePhaseFromHealth(plane)

			Expect(phase).Should(Equal(v1alpha1.PlanePhaseDegraded))
		})

		It("should return Publishing when components pending", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					HealthSummary: &v1alpha1.PlaneHealthSummary{
						TotalComponents:   3,
						PendingComponents: 1,
					},
				},
			}

			phase := DeterminePhaseFromHealth(plane)

			Expect(phase).Should(Equal(v1alpha1.PlanePhasePublishing))
		})

		It("should return Running when all healthy", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					HealthSummary: &v1alpha1.PlaneHealthSummary{
						TotalComponents:   3,
						HealthyComponents: 3,
						OverallHealthy:    true,
					},
				},
			}

			phase := DeterminePhaseFromHealth(plane)

			Expect(phase).Should(Equal(v1alpha1.PlanePhaseRunning))
		})
	})

	Describe("SetComponent helpers", func() {
		var plane *v1alpha1.ClusterPlane

		BeforeEach(func() {
			plane = &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "comp-a", Phase: v1alpha1.ComponentPhasePending},
					},
				},
			}
		})

		It("SetComponentDeploying should set Deploying phase", func() {
			changed := SetComponentDeploying(plane, "comp-a")

			Expect(changed).Should(BeTrue())
			cs := GetComponentStatus(plane, "comp-a")
			Expect(cs.Phase).Should(Equal(v1alpha1.ComponentPhaseDeploying))
			Expect(cs.Healthy).Should(BeFalse())
		})

		It("SetComponentRunning should set Running phase and healthy", func() {
			changed := SetComponentRunning(plane, "comp-a")

			Expect(changed).Should(BeTrue())
			cs := GetComponentStatus(plane, "comp-a")
			Expect(cs.Phase).Should(Equal(v1alpha1.ComponentPhaseRunning))
			Expect(cs.Healthy).Should(BeTrue())
		})

		It("SetComponentDegraded should set Degraded phase", func() {
			changed := SetComponentDegraded(plane, "comp-a", "Memory pressure")

			Expect(changed).Should(BeTrue())
			cs := GetComponentStatus(plane, "comp-a")
			Expect(cs.Phase).Should(Equal(v1alpha1.ComponentPhaseDegraded))
			Expect(cs.Healthy).Should(BeFalse())
			Expect(cs.Message).Should(Equal("Memory pressure"))
		})

		It("SetComponentFailed should set Failed phase", func() {
			changed := SetComponentFailed(plane, "comp-a", "Fatal error")

			Expect(changed).Should(BeTrue())
			cs := GetComponentStatus(plane, "comp-a")
			Expect(cs.Phase).Should(Equal(v1alpha1.ComponentPhaseFailed))
			Expect(cs.Healthy).Should(BeFalse())
			Expect(cs.Message).Should(Equal("Fatal error"))
		})
	})

	Describe("SetTraitStatus", func() {
		It("should add new trait status", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "comp-a"},
					},
				},
			}

			SetTraitStatus(plane, "comp-a", "scaling", true, "Scaling configured")

			cs := GetComponentStatus(plane, "comp-a")
			Expect(cs.TraitStatuses).Should(HaveLen(1))
			Expect(cs.TraitStatuses[0].Type).Should(Equal("scaling"))
			Expect(cs.TraitStatuses[0].Healthy).Should(BeTrue())
		})

		It("should update existing trait status", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{
							Name: "comp-a",
							TraitStatuses: []v1alpha1.TraitHealthStatus{
								{Type: "scaling", Healthy: true, Message: "OK"},
							},
						},
					},
				},
			}

			SetTraitStatus(plane, "comp-a", "scaling", false, "Failed")

			cs := GetComponentStatus(plane, "comp-a")
			Expect(cs.TraitStatuses).Should(HaveLen(1))
			Expect(cs.TraitStatuses[0].Healthy).Should(BeFalse())
			Expect(cs.TraitStatuses[0].Message).Should(Equal("Failed"))
		})

		It("should add multiple trait statuses", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "comp-a"},
					},
				},
			}

			SetTraitStatus(plane, "comp-a", "scaling", true, "OK")
			SetTraitStatus(plane, "comp-a", "gateway", true, "Ready")

			cs := GetComponentStatus(plane, "comp-a")
			Expect(cs.TraitStatuses).Should(HaveLen(2))
		})
	})

	Describe("SetComponentDetail", func() {
		It("should initialize details map and set value", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "comp-a"},
					},
				},
			}

			SetComponentDetail(plane, "comp-a", "version", "1.0.0")

			cs := GetComponentStatus(plane, "comp-a")
			Expect(cs.Details).ShouldNot(BeNil())
			Expect(cs.Details["version"]).Should(Equal("1.0.0"))
		})

		It("should add multiple details", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{
						{Name: "comp-a"},
					},
				},
			}

			SetComponentDetail(plane, "comp-a", "version", "1.0.0")
			SetComponentDetail(plane, "comp-a", "namespace", "default")

			cs := GetComponentStatus(plane, "comp-a")
			Expect(cs.Details).Should(HaveLen(2))
		})

		It("should be no-op for non-existent component", func() {
			plane := &v1alpha1.ClusterPlane{
				Status: v1alpha1.ClusterPlaneStatus{
					ComponentHealth: []v1alpha1.ComponentHealthStatus{},
				},
			}

			// Should not panic
			SetComponentDetail(plane, "nonexistent", "key", "value")
		})
	})

	Describe("ReconcileComponentStatuses", func() {
		It("should initialize and calculate health summary", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-plane",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "comp-a", Type: "helm"},
						{Name: "comp-b", Type: "kustomize"},
					},
				},
			}

			ReconcileComponentStatuses(plane)

			Expect(plane.Status.ComponentHealth).Should(HaveLen(2))
			Expect(plane.Status.HealthSummary).ShouldNot(BeNil())
			Expect(plane.Status.HealthSummary.TotalComponents).Should(Equal(2))
			Expect(plane.Status.HealthSummary.PendingComponents).Should(Equal(2))
			Expect(plane.Status.HealthSummary.OverallHealthy).Should(BeFalse())
		})

		It("should update summary after status changes", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "comp-a", Type: "helm"},
						{Name: "comp-b", Type: "kustomize"},
					},
				},
			}

			ReconcileComponentStatuses(plane)
			SetComponentRunning(plane, "comp-a")
			SetComponentRunning(plane, "comp-b")
			ReconcileComponentStatuses(plane)

			Expect(plane.Status.HealthSummary.HealthyComponents).Should(Equal(2))
			Expect(plane.Status.HealthSummary.PendingComponents).Should(Equal(0))
			Expect(plane.Status.HealthSummary.OverallHealthy).Should(BeTrue())
		})

		It("should reflect degraded state in summary", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "comp-a", Type: "helm"},
						{Name: "comp-b", Type: "kustomize"},
					},
				},
			}

			ReconcileComponentStatuses(plane)
			SetComponentRunning(plane, "comp-a")
			SetComponentDegraded(plane, "comp-b", "Memory pressure")
			ReconcileComponentStatuses(plane)

			Expect(plane.Status.HealthSummary.HealthyComponents).Should(Equal(1))
			Expect(plane.Status.HealthSummary.DegradedComponents).Should(Equal(1))
			Expect(plane.Status.HealthSummary.OverallHealthy).Should(BeFalse())
		})
	})

	Describe("phaseToMetricValue", func() {
		DescribeTable("should return correct metric values",
			func(phase v1alpha1.PlanePhase, expected float64) {
				Expect(phaseToMetricValue(phase)).Should(Equal(expected))
			},
			Entry("Draft", v1alpha1.PlanePhaseDraft, float64(0)),
			Entry("Publishing", v1alpha1.PlanePhasePublishing, float64(1)),
			Entry("Running", v1alpha1.PlanePhaseRunning, float64(2)),
			Entry("Degraded", v1alpha1.PlanePhaseDegraded, float64(3)),
			Entry("Suspended", v1alpha1.PlanePhaseSuspended, float64(4)),
			Entry("Failed", v1alpha1.PlanePhaseFailed, float64(5)),
			Entry("Unknown", v1alpha1.PlanePhase("unknown"), float64(-1)),
		)
	})
})

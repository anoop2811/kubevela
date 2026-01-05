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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("ClusterPlane Controller Test", func() {
	ctx := context.Background()

	Context("When creating a ClusterPlane without publishVersion annotation", func() {
		var planeName = "test-plane-draft"
		var namespace = "ns-cp-test-1"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: planeName, Namespace: namespace}}

		It("should create a ClusterPlane in Draft phase", func() {
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Create a ClusterPlane without publishVersion annotation")
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      planeName,
					Namespace: namespace,
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Description: "Test cluster plane for networking",
					Components: []v1alpha1.PlaneComponent{
						{
							Name: "calico",
							Type: "helm",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"chart": "calico", "version": "3.26.0"}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, plane)).Should(Succeed())

			By("Reconcile the ClusterPlane")
			ReconcileRetry(&r, req)

			By("Verify the ClusterPlane is in Draft phase")
			var gotPlane v1alpha1.ClusterPlane
			Eventually(func() v1alpha1.PlanePhase {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: planeName}, &gotPlane)
				if err != nil {
					return ""
				}
				return gotPlane.Status.Phase
			}, 30*time.Second, time.Second).Should(Equal(v1alpha1.PlanePhaseDraft))

			By("Verify observedGeneration is updated")
			Expect(gotPlane.Status.ObservedGeneration).Should(Equal(gotPlane.Generation))

			By("Verify the Reconciled condition is set")
			cond := gotPlane.GetCondition(ConditionTypeReconciled)
			Expect(cond.Status).Should(Equal("True"))
			Expect(cond.Reason).Should(Equal("Draft"))

			By("Delete the ClusterPlane")
			Expect(k8sClient.Delete(ctx, plane)).Should(Succeed())
		})
	})

	Context("When creating a ClusterPlane with publishVersion annotation", func() {
		var planeName = "test-plane-publishing"
		var namespace = "ns-cp-test-2"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: planeName, Namespace: namespace}}

		It("should create a ClusterPlane in Publishing phase", func() {
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Create a ClusterPlane with publishVersion annotation")
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      planeName,
					Namespace: namespace,
					Annotations: map[string]string{
						AnnotationPublishVersion: "v1",
					},
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Description: "Test cluster plane for security",
					Components: []v1alpha1.PlaneComponent{
						{
							Name: "cert-manager",
							Type: "helm",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"chart": "cert-manager", "version": "1.13.0"}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, plane)).Should(Succeed())

			By("Reconcile the ClusterPlane")
			ReconcileRetry(&r, req)

			By("Verify the ClusterPlane is in Publishing phase (no revision yet)")
			var gotPlane v1alpha1.ClusterPlane
			Eventually(func() v1alpha1.PlanePhase {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: planeName}, &gotPlane)
				if err != nil {
					return ""
				}
				return gotPlane.Status.Phase
			}, 30*time.Second, time.Second).Should(Equal(v1alpha1.PlanePhasePublishing))

			By("Delete the ClusterPlane")
			Expect(k8sClient.Delete(ctx, plane)).Should(Succeed())
		})
	})

	Context("When updating a ClusterPlane", func() {
		var planeName = "test-plane-update"
		var namespace = "ns-cp-test-3"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: planeName, Namespace: namespace}}

		It("should transition from Draft to Publishing when annotation is added", func() {
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Create a ClusterPlane without publishVersion annotation")
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      planeName,
					Namespace: namespace,
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Description: "Test cluster plane for storage",
					Components: []v1alpha1.PlaneComponent{
						{
							Name: "rook-ceph",
							Type: "helm",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"chart": "rook-ceph", "version": "1.12.0"}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, plane)).Should(Succeed())

			By("Reconcile the ClusterPlane")
			ReconcileRetry(&r, req)

			By("Verify the ClusterPlane is in Draft phase")
			var gotPlane v1alpha1.ClusterPlane
			Eventually(func() v1alpha1.PlanePhase {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: planeName}, &gotPlane)
				if err != nil {
					return ""
				}
				return gotPlane.Status.Phase
			}, 30*time.Second, time.Second).Should(Equal(v1alpha1.PlanePhaseDraft))

			By("Add the publishVersion annotation")
			gotPlane.Annotations = map[string]string{
				AnnotationPublishVersion: "v1",
			}
			Expect(k8sClient.Update(ctx, &gotPlane)).Should(Succeed())

			By("Reconcile the ClusterPlane again")
			ReconcileRetry(&r, req)

			By("Verify the ClusterPlane transitioned to Publishing phase")
			Eventually(func() v1alpha1.PlanePhase {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: planeName}, &gotPlane)
				if err != nil {
					return ""
				}
				return gotPlane.Status.Phase
			}, 30*time.Second, time.Second).Should(Equal(v1alpha1.PlanePhasePublishing))

			By("Delete the ClusterPlane")
			Expect(k8sClient.Delete(ctx, &gotPlane)).Should(Succeed())
		})
	})

	Context("When the ClusterPlane is deleted", func() {
		var planeName = "test-plane-delete"
		var namespace = "ns-cp-test-4"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: planeName, Namespace: namespace}}

		It("should clean up metrics when ClusterPlane is deleted", func() {
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Create a ClusterPlane")
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      planeName,
					Namespace: namespace,
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Description: "Test cluster plane for observability",
					Components: []v1alpha1.PlaneComponent{
						{
							Name: "prometheus",
							Type: "helm",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"chart": "prometheus", "version": "25.0.0"}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, plane)).Should(Succeed())

			By("Reconcile the ClusterPlane")
			ReconcileRetry(&r, req)

			By("Verify the ClusterPlane exists")
			var gotPlane v1alpha1.ClusterPlane
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: planeName}, &gotPlane)).Should(Succeed())

			By("Delete the ClusterPlane")
			Expect(k8sClient.Delete(ctx, &gotPlane)).Should(Succeed())

			By("Reconcile after deletion")
			ReconcileRetry(&r, req)

			By("Verify the ClusterPlane no longer exists")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: planeName}, &gotPlane)
				return err != nil
			}, 30*time.Second, time.Second).Should(BeTrue())
		})
	})
})

var _ = Describe("phaseToMetricValue Function Test", func() {
	It("should return correct metric values for each phase", func() {
		Expect(phaseToMetricValue(v1alpha1.PlanePhaseDraft)).Should(Equal(float64(0)))
		Expect(phaseToMetricValue(v1alpha1.PlanePhasePublishing)).Should(Equal(float64(1)))
		Expect(phaseToMetricValue(v1alpha1.PlanePhaseRunning)).Should(Equal(float64(2)))
		Expect(phaseToMetricValue(v1alpha1.PlanePhaseSuspended)).Should(Equal(float64(3)))
		Expect(phaseToMetricValue(v1alpha1.PlanePhaseFailed)).Should(Equal(float64(4)))
		Expect(phaseToMetricValue(v1alpha1.PlanePhase("Unknown"))).Should(Equal(float64(-1)))
	})
})

var _ = Describe("determinePhase Function Test", func() {
	It("should return Draft when no publishVersion annotation", func() {
		plane := &v1alpha1.ClusterPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-plane",
				Namespace: "default",
			},
		}
		phase := r.determinePhase(plane)
		Expect(phase).Should(Equal(v1alpha1.PlanePhaseDraft))
	})

	It("should return Publishing when publishVersion is set but no current revision", func() {
		plane := &v1alpha1.ClusterPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-plane",
				Namespace: "default",
				Annotations: map[string]string{
					AnnotationPublishVersion: "v1",
				},
			},
		}
		phase := r.determinePhase(plane)
		Expect(phase).Should(Equal(v1alpha1.PlanePhasePublishing))
	})

	It("should return Running when publishVersion is set and current revision exists", func() {
		plane := &v1alpha1.ClusterPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-plane",
				Namespace: "default",
				Annotations: map[string]string{
					AnnotationPublishVersion: "v1",
				},
			},
			Status: v1alpha1.ClusterPlaneStatus{
				CurrentRevision: &v1alpha1.RevisionReference{
					Name:    "test-plane-v1",
					Version: "v1",
				},
			},
		}
		phase := r.determinePhase(plane)
		Expect(phase).Should(Equal(v1alpha1.PlanePhaseRunning))
	})
})

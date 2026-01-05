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

var _ = Describe("ClusterPlaneRevision Tests", func() {
	ctx := context.Background()

	Context("Revision Creation Tests", func() {
		var planeName = "test-plane-revision-create"
		var namespace = "ns-revision-test-1"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: planeName, Namespace: namespace}}

		It("should create a revision when publishVersion annotation is added", func() {
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
						AnnotationPublishVersion: "v1.0.0",
					},
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Description: "Test cluster plane for revision creation",
					Components: []v1alpha1.PlaneComponent{
						{
							Name: "nginx-ingress",
							Type: "helm",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"chart": "nginx-ingress", "version": "4.0.0"}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, plane)).Should(Succeed())

			By("Reconcile the ClusterPlane")
			ReconcileRetry(&r, req)

			By("Verify a ClusterPlaneRevision was created")
			var revisionList v1alpha1.ClusterPlaneRevisionList
			Eventually(func() int {
				err := k8sClient.List(ctx, &revisionList,
					client.InNamespace(namespace),
					client.MatchingLabels{LabelClusterPlaneName: planeName})
				if err != nil {
					return 0
				}
				return len(revisionList.Items)
			}, 30*time.Second, time.Second).Should(BeNumerically(">=", 1))

			By("Verify the revision has correct metadata")
			revision := revisionList.Items[0]
			Expect(revision.Labels[LabelClusterPlaneName]).Should(Equal(planeName))
			Expect(revision.Labels[LabelClusterPlaneRevisionHash]).ShouldNot(BeEmpty())
			Expect(revision.Spec.PlaneSnapshot.Version).Should(Equal("v1.0.0"))
			Expect(len(revision.Spec.PlaneSnapshot.Components)).Should(Equal(1))
			Expect(revision.Spec.PlaneSnapshot.Components[0].Name).Should(Equal("nginx-ingress"))

			By("Verify the revision has owner reference to the ClusterPlane")
			Expect(len(revision.OwnerReferences)).Should(Equal(1))
			Expect(revision.OwnerReferences[0].Name).Should(Equal(planeName))
			Expect(revision.OwnerReferences[0].Kind).Should(Equal("ClusterPlane"))

			By("Verify the ClusterPlane status is updated with current revision")
			var gotPlane v1alpha1.ClusterPlane
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: planeName}, &gotPlane)
				if err != nil || gotPlane.Status.CurrentRevision == nil {
					return ""
				}
				return gotPlane.Status.CurrentRevision.Name
			}, 30*time.Second, time.Second).ShouldNot(BeEmpty())

			Expect(gotPlane.Status.CurrentRevision.Version).Should(Equal("v1.0.0"))

			By("Verify the ClusterPlane is now in Running phase")
			Eventually(func() v1alpha1.PlanePhase {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: planeName}, &gotPlane)
				if err != nil {
					return ""
				}
				return gotPlane.Status.Phase
			}, 30*time.Second, time.Second).Should(Equal(v1alpha1.PlanePhaseRunning))

			By("Delete the ClusterPlane")
			Expect(k8sClient.Delete(ctx, plane)).Should(Succeed())
		})
	})

	Context("Revision Deduplication Tests", func() {
		var planeName = "test-plane-revision-dedup"
		var namespace = "ns-revision-test-2"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: planeName, Namespace: namespace}}

		It("should not create duplicate revisions for the same spec", func() {
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
						AnnotationPublishVersion: "v1.0.0",
					},
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Description: "Test cluster plane for deduplication",
					Components: []v1alpha1.PlaneComponent{
						{
							Name: "istio",
							Type: "helm",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"chart": "istio", "version": "1.20.0"}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, plane)).Should(Succeed())

			By("Reconcile the ClusterPlane multiple times")
			ReconcileRetry(&r, req)
			ReconcileRetry(&r, req)
			ReconcileRetry(&r, req)

			By("Verify only one revision was created (due to same hash)")
			var revisionList v1alpha1.ClusterPlaneRevisionList
			Eventually(func() int {
				err := k8sClient.List(ctx, &revisionList,
					client.InNamespace(namespace),
					client.MatchingLabels{LabelClusterPlaneName: planeName})
				if err != nil {
					return -1
				}
				return len(revisionList.Items)
			}, 30*time.Second, time.Second).Should(Equal(1))

			By("Delete the ClusterPlane")
			Expect(k8sClient.Delete(ctx, plane)).Should(Succeed())
		})
	})

	Context("Revision Version Update Tests", func() {
		var planeName = "test-plane-revision-version"
		var namespace = "ns-revision-test-3"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: planeName, Namespace: namespace}}

		It("should create new revision when version changes", func() {
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Create a ClusterPlane with publishVersion v1")
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      planeName,
					Namespace: namespace,
					Annotations: map[string]string{
						AnnotationPublishVersion: "v1",
					},
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Description: "Test cluster plane for version updates",
					Components: []v1alpha1.PlaneComponent{
						{
							Name: "kiali",
							Type: "helm",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"chart": "kiali", "version": "1.0.0"}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, plane)).Should(Succeed())

			By("Reconcile the ClusterPlane")
			ReconcileRetry(&r, req)

			By("Verify first revision was created")
			var revisionList v1alpha1.ClusterPlaneRevisionList
			Eventually(func() int {
				err := k8sClient.List(ctx, &revisionList,
					client.InNamespace(namespace),
					client.MatchingLabels{LabelClusterPlaneName: planeName})
				if err != nil {
					return 0
				}
				return len(revisionList.Items)
			}, 30*time.Second, time.Second).Should(Equal(1))

			By("Update the publish version to v2")
			var gotPlane v1alpha1.ClusterPlane
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: planeName}, &gotPlane)).Should(Succeed())
			gotPlane.Annotations[AnnotationPublishVersion] = "v2"
			Expect(k8sClient.Update(ctx, &gotPlane)).Should(Succeed())

			By("Reconcile after version update")
			ReconcileRetry(&r, req)

			By("Verify a new revision was created (different version = different hash)")
			Eventually(func() int {
				err := k8sClient.List(ctx, &revisionList,
					client.InNamespace(namespace),
					client.MatchingLabels{LabelClusterPlaneName: planeName})
				if err != nil {
					return 0
				}
				return len(revisionList.Items)
			}, 30*time.Second, time.Second).Should(Equal(2))

			By("Verify revisions have different versions")
			versions := make(map[string]bool)
			for _, rev := range revisionList.Items {
				versions[rev.Spec.PlaneSnapshot.Version] = true
			}
			Expect(versions).Should(HaveKey("v1"))
			Expect(versions).Should(HaveKey("v2"))

			By("Delete the ClusterPlane")
			Expect(k8sClient.Delete(ctx, &gotPlane)).Should(Succeed())
		})
	})

	Context("Revision Cleanup Tests", func() {
		var planeName = "test-plane-revision-cleanup"
		var namespace = "ns-revision-test-4"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: planeName, Namespace: namespace}}

		It("should clean up old revisions beyond the limit", func() {
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
					Annotations: map[string]string{
						AnnotationPublishVersion: "v1",
					},
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Description: "Test cluster plane for cleanup",
					Components: []v1alpha1.PlaneComponent{
						{
							Name: "grafana",
							Type: "helm",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"chart": "grafana", "version": "1.0.0"}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, plane)).Should(Succeed())

			By("Create multiple revisions by changing version")
			for i := 1; i <= 15; i++ {
				var gotPlane v1alpha1.ClusterPlane
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: planeName}, &gotPlane)).Should(Succeed())
				gotPlane.Annotations[AnnotationPublishVersion] = fmt.Sprintf("v%d", i)
				Expect(k8sClient.Update(ctx, &gotPlane)).Should(Succeed())
				ReconcileRetry(&r, req)
				// Small sleep to ensure distinct creation timestamps
				time.Sleep(100 * time.Millisecond)
			}

			By("Verify revisions are cleaned up to the limit")
			var revisionList v1alpha1.ClusterPlaneRevisionList
			Eventually(func() int {
				err := k8sClient.List(ctx, &revisionList,
					client.InNamespace(namespace),
					client.MatchingLabels{LabelClusterPlaneName: planeName})
				if err != nil {
					return -1
				}
				return len(revisionList.Items)
			}, 60*time.Second, time.Second).Should(BeNumerically("<=", defRevisionLimit+1))

			By("Delete the ClusterPlane")
			var gotPlane v1alpha1.ClusterPlane
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: planeName}, &gotPlane)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, &gotPlane)).Should(Succeed())
		})
	})
})

var _ = Describe("ComputePlaneRevisionHash Function Test", func() {
	It("should return same hash for identical snapshots", func() {
		snapshot1 := &v1alpha1.PlaneSnapshot{
			Version: "v1.0.0",
			Components: []v1alpha1.PlaneComponent{
				{
					Name: "test",
					Type: "helm",
				},
			},
		}
		snapshot2 := &v1alpha1.PlaneSnapshot{
			Version: "v1.0.0",
			Components: []v1alpha1.PlaneComponent{
				{
					Name: "test",
					Type: "helm",
				},
			},
		}

		hash1, err1 := ComputePlaneRevisionHash(snapshot1)
		hash2, err2 := ComputePlaneRevisionHash(snapshot2)

		Expect(err1).ShouldNot(HaveOccurred())
		Expect(err2).ShouldNot(HaveOccurred())
		Expect(hash1).Should(Equal(hash2))
	})

	It("should return different hash for different versions", func() {
		snapshot1 := &v1alpha1.PlaneSnapshot{
			Version: "v1.0.0",
		}
		snapshot2 := &v1alpha1.PlaneSnapshot{
			Version: "v2.0.0",
		}

		hash1, err1 := ComputePlaneRevisionHash(snapshot1)
		hash2, err2 := ComputePlaneRevisionHash(snapshot2)

		Expect(err1).ShouldNot(HaveOccurred())
		Expect(err2).ShouldNot(HaveOccurred())
		Expect(hash1).ShouldNot(Equal(hash2))
	})

	It("should return different hash for different components", func() {
		snapshot1 := &v1alpha1.PlaneSnapshot{
			Version: "v1.0.0",
			Components: []v1alpha1.PlaneComponent{
				{Name: "component-a", Type: "helm"},
			},
		}
		snapshot2 := &v1alpha1.PlaneSnapshot{
			Version: "v1.0.0",
			Components: []v1alpha1.PlaneComponent{
				{Name: "component-b", Type: "helm"},
			},
		}

		hash1, err1 := ComputePlaneRevisionHash(snapshot1)
		hash2, err2 := ComputePlaneRevisionHash(snapshot2)

		Expect(err1).ShouldNot(HaveOccurred())
		Expect(err2).ShouldNot(HaveOccurred())
		Expect(hash1).ShouldNot(Equal(hash2))
	})
})

var _ = Describe("GetRevisionReference Function Test", func() {
	It("should create correct RevisionReference from ClusterPlaneRevision", func() {
		revision := &v1alpha1.ClusterPlaneRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-plane-v1",
				Namespace: "default",
			},
			Spec: v1alpha1.ClusterPlaneRevisionSpec{
				PlaneSnapshot: v1alpha1.PlaneSnapshot{
					Version: "v1.0.0",
				},
			},
		}

		ref := GetRevisionReference(revision)

		Expect(ref.Name).Should(Equal("test-plane-v1"))
		Expect(ref.Version).Should(Equal("v1.0.0"))
	})
})

var _ = Describe("generatePlaneRevisionSpec Function Test", func() {
	It("should create PlaneSnapshot with all fields copied", func() {
		plane := &v1alpha1.ClusterPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-plane",
				Namespace: "default",
			},
			Spec: v1alpha1.ClusterPlaneSpec{
				Owner: &v1alpha1.OwnerInfo{
					Team:     "platform",
					Contacts: []string{"platform@example.com"},
				},
				Components: []v1alpha1.PlaneComponent{
					{Name: "comp1", Type: "helm"},
					{Name: "comp2", Type: "kustomize"},
				},
				Traits: []v1alpha1.PlaneTrait{
					{Type: "scaler"},
				},
				Policies: []v1alpha1.PlanePolicy{
					{Name: "policy1", Type: "topology"},
				},
				Outputs: []v1alpha1.PlaneOutput{
					{Name: "output1", ValueFrom: v1alpha1.PlaneOutputValueFrom{Component: "comp1", FieldPath: "status.endpoint"}},
				},
			},
		}

		snapshot := generatePlaneRevisionSpec(plane, "v1.0.0")

		Expect(snapshot.Version).Should(Equal("v1.0.0"))
		Expect(snapshot.Owner).ShouldNot(BeNil())
		Expect(snapshot.Owner.Team).Should(Equal("platform"))
		Expect(len(snapshot.Owner.Contacts)).Should(Equal(1))
		Expect(len(snapshot.Components)).Should(Equal(2))
		Expect(len(snapshot.Traits)).Should(Equal(1))
		Expect(len(snapshot.Policies)).Should(Equal(1))
		Expect(len(snapshot.Outputs)).Should(Equal(1))
	})

	It("should handle nil owner gracefully", func() {
		plane := &v1alpha1.ClusterPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-plane",
				Namespace: "default",
			},
			Spec: v1alpha1.ClusterPlaneSpec{
				Components: []v1alpha1.PlaneComponent{
					{Name: "comp1", Type: "helm"},
				},
			},
		}

		snapshot := generatePlaneRevisionSpec(plane, "v1.0.0")

		Expect(snapshot.Version).Should(Equal("v1.0.0"))
		Expect(snapshot.Owner).Should(BeNil())
		Expect(len(snapshot.Components)).Should(Equal(1))
	})
})

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("PlaneResourceManager", func() {
	var (
		ctx        context.Context
		k8sClient  client.Client
		manager    *PlaneResourceManager
		plane      *v1alpha1.ClusterPlane
		testNS     string
	)

	BeforeEach(func() {
		ctx = context.Background()
		k8sClient = k8sClient // Use the client from suite_test.go
		manager = NewPlaneResourceManager(k8sClient)

		// Use a unique namespace for each test
		testNS = "resource-manager-test"

		// Ensure test namespace exists
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNS,
			},
		}
		err := k8sClient.Create(ctx, ns)
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}

		plane = &v1alpha1.ClusterPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-plane",
				Namespace:  testNS,
				UID:        types.UID("test-uid-12345"),
				Generation: 1,
			},
		}
	})

	AfterEach(func() {
		// Clean up ResourceTracker
		rt := &corev1beta1.ResourceTracker{}
		rtName := GetResourceTrackerName(plane)
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: rtName}, rt); err == nil {
			k8sClient.Delete(ctx, rt)
		}

		// Clean up test ConfigMaps
		configMaps := &corev1.ConfigMapList{}
		k8sClient.List(ctx, configMaps, client.InNamespace(testNS), client.MatchingLabels{
			LabelManagedBy: ManagedByClusterPlane,
		})
		for _, cm := range configMaps.Items {
			k8sClient.Delete(ctx, &cm)
		}
	})

	Describe("GetResourceTrackerName", func() {
		It("should generate consistent ResourceTracker names", func() {
			name := GetResourceTrackerName(plane)
			Expect(name).To(Equal("clusterplane-resource-manager-test-test-plane"))
		})

		It("should include namespace to avoid conflicts", func() {
			plane1 := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-plane",
					Namespace: "ns1",
				},
			}
			plane2 := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-plane",
					Namespace: "ns2",
				},
			}

			Expect(GetResourceTrackerName(plane1)).NotTo(Equal(GetResourceTrackerName(plane2)))
		})
	})

	Describe("EnsureResourceTracker", func() {
		It("should create a new ResourceTracker if not exists", func() {
			rt, err := manager.EnsureResourceTracker(ctx, plane)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt).NotTo(BeNil())
			Expect(rt.Name).To(Equal(GetResourceTrackerName(plane)))

			// Verify labels
			Expect(rt.Labels[LabelPlaneName]).To(Equal(plane.Name))
			Expect(rt.Labels[LabelPlaneNamespace]).To(Equal(plane.Namespace))
			Expect(rt.Labels[LabelManagedBy]).To(Equal(ManagedByClusterPlane))

			// Verify owner reference
			Expect(rt.OwnerReferences).To(HaveLen(1))
			Expect(rt.OwnerReferences[0].Name).To(Equal(plane.Name))
			Expect(rt.OwnerReferences[0].Kind).To(Equal("ClusterPlane"))
		})

		It("should return existing ResourceTracker if already exists", func() {
			// First call creates
			rt1, err := manager.EnsureResourceTracker(ctx, plane)
			Expect(err).NotTo(HaveOccurred())

			// Second call returns existing
			rt2, err := manager.EnsureResourceTracker(ctx, plane)
			Expect(err).NotTo(HaveOccurred())

			Expect(rt1.UID).To(Equal(rt2.UID))
		})
	})

	Describe("DispatchResources", func() {
		It("should create resources and track them", func() {
			resources := []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "test-cm-1",
							"namespace": testNS,
							"labels": map[string]interface{}{
								LabelPlaneName:      plane.Name,
								LabelPlaneNamespace: plane.Namespace,
								LabelManagedBy:      ManagedByClusterPlane,
							},
						},
						"data": map[string]interface{}{
							"key": "value",
						},
					},
				},
			}

			results, err := manager.DispatchResources(ctx, plane, resources)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results[0].Created).To(BeTrue())
			Expect(results[0].Error).NotTo(HaveOccurred())

			// Verify ConfigMap was created
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-cm-1",
				Namespace: testNS,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data["key"]).To(Equal("value"))

			// Verify ResourceTracker tracks the resource
			rt, err := manager.GetResourceTracker(ctx, plane)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.Spec.ManagedResources).To(HaveLen(1))
		})

		It("should update existing resources", func() {
			// Create initial ConfigMap
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-test-cm",
					Namespace: testNS,
					Labels: map[string]string{
						LabelPlaneName:      plane.Name,
						LabelPlaneNamespace: plane.Namespace,
						LabelManagedBy:      ManagedByClusterPlane,
					},
				},
				Data: map[string]string{"key": "original"},
			}
			err := k8sClient.Create(ctx, cm)
			Expect(err).NotTo(HaveOccurred())

			// Dispatch with updated value
			resources := []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "update-test-cm",
							"namespace": testNS,
							"labels": map[string]interface{}{
								LabelPlaneName:      plane.Name,
								LabelPlaneNamespace: plane.Namespace,
								LabelManagedBy:      ManagedByClusterPlane,
							},
						},
						"data": map[string]interface{}{
							"key": "updated",
						},
					},
				},
			}

			results, err := manager.DispatchResources(ctx, plane, resources)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results[0].Updated).To(BeTrue())
			Expect(results[0].Created).To(BeFalse())

			// Verify update
			updatedCM := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "update-test-cm",
				Namespace: testNS,
			}, updatedCM)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedCM.Data["key"]).To(Equal("updated"))
		})

		It("should reject updating resources not managed by this plane", func() {
			// Create ConfigMap owned by different plane
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foreign-cm",
					Namespace: testNS,
					Labels: map[string]string{
						LabelPlaneName:      "other-plane",
						LabelPlaneNamespace: testNS,
						LabelManagedBy:      ManagedByClusterPlane,
					},
				},
				Data: map[string]string{"key": "original"},
			}
			err := k8sClient.Create(ctx, cm)
			Expect(err).NotTo(HaveOccurred())

			// Try to dispatch from our plane
			resources := []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "foreign-cm",
							"namespace": testNS,
							"labels": map[string]interface{}{
								LabelPlaneName:      plane.Name, // Different plane
								LabelPlaneNamespace: plane.Namespace,
								LabelManagedBy:      ManagedByClusterPlane,
							},
						},
						"data": map[string]interface{}{
							"key": "hijacked",
						},
					},
				},
			}

			results, err := manager.DispatchResources(ctx, plane, resources)
			Expect(err).NotTo(HaveOccurred()) // Overall dispatch doesn't fail
			Expect(results).To(HaveLen(1))
			Expect(results[0].Error).To(HaveOccurred()) // But individual resource dispatch fails
			Expect(results[0].Error.Error()).To(ContainSubstring("not managed by this ClusterPlane"))

			// Verify ConfigMap wasn't modified
			existingCM := &corev1.ConfigMap{}
			k8sClient.Get(ctx, types.NamespacedName{Name: "foreign-cm", Namespace: testNS}, existingCM)
			Expect(existingCM.Data["key"]).To(Equal("original"))
		})
	})

	Describe("GarbageCollect", func() {
		It("should delete resources no longer in desired state", func() {
			// First, dispatch a resource
			resources := []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "gc-test-cm",
							"namespace": testNS,
							"labels": map[string]interface{}{
								LabelPlaneName:      plane.Name,
								LabelPlaneNamespace: plane.Namespace,
								LabelManagedBy:      ManagedByClusterPlane,
							},
						},
					},
				},
			}

			_, err := manager.DispatchResources(ctx, plane, resources)
			Expect(err).NotTo(HaveOccurred())

			// Verify it exists
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "gc-test-cm", Namespace: testNS}, cm)
			Expect(err).NotTo(HaveOccurred())

			// Now garbage collect with empty desired state
			err = manager.GarbageCollect(ctx, plane, []*unstructured.Unstructured{})
			Expect(err).NotTo(HaveOccurred())

			// Verify it was deleted
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "gc-test-cm", Namespace: testNS}, cm)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should not delete resources still in desired state", func() {
			resource := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "keep-cm",
						"namespace": testNS,
						"labels": map[string]interface{}{
							LabelPlaneName:      plane.Name,
							LabelPlaneNamespace: plane.Namespace,
							LabelManagedBy:      ManagedByClusterPlane,
						},
					},
				},
			}

			_, err := manager.DispatchResources(ctx, plane, []*unstructured.Unstructured{resource})
			Expect(err).NotTo(HaveOccurred())

			// GC with the same resource in desired state
			err = manager.GarbageCollect(ctx, plane, []*unstructured.Unstructured{resource})
			Expect(err).NotTo(HaveOccurred())

			// Should still exist
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "keep-cm", Namespace: testNS}, cm)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("GetManagedResources", func() {
		It("should return empty list when no ResourceTracker exists", func() {
			resources, err := manager.GetManagedResources(ctx, plane)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources).To(BeNil())
		})

		It("should return managed resources from ResourceTracker", func() {
			// Dispatch some resources
			dispatchResources := []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "managed-cm-1",
							"namespace": testNS,
							"labels": map[string]interface{}{
								LabelPlaneName:      plane.Name,
								LabelPlaneNamespace: plane.Namespace,
								LabelManagedBy:      ManagedByClusterPlane,
							},
						},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "managed-cm-2",
							"namespace": testNS,
							"labels": map[string]interface{}{
								LabelPlaneName:      plane.Name,
								LabelPlaneNamespace: plane.Namespace,
								LabelManagedBy:      ManagedByClusterPlane,
							},
						},
					},
				},
			}

			_, err := manager.DispatchResources(ctx, plane, dispatchResources)
			Expect(err).NotTo(HaveOccurred())

			// Get managed resources
			resources, err := manager.GetManagedResources(ctx, plane)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources).To(HaveLen(2))
		})
	})

	Describe("DeleteManagedResources", func() {
		It("should delete all managed resources", func() {
			// Dispatch resources
			resources := []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "delete-test-1",
							"namespace": testNS,
							"labels": map[string]interface{}{
								LabelPlaneName:      plane.Name,
								LabelPlaneNamespace: plane.Namespace,
								LabelManagedBy:      ManagedByClusterPlane,
							},
						},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":      "delete-test-2",
							"namespace": testNS,
							"labels": map[string]interface{}{
								LabelPlaneName:      plane.Name,
								LabelPlaneNamespace: plane.Namespace,
								LabelManagedBy:      ManagedByClusterPlane,
							},
						},
					},
				},
			}

			_, err := manager.DispatchResources(ctx, plane, resources)
			Expect(err).NotTo(HaveOccurred())

			// Delete all managed resources
			err = manager.DeleteManagedResources(ctx, plane)
			Expect(err).NotTo(HaveOccurred())

			// Verify resources are deleted
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "delete-test-1", Namespace: testNS}, cm)
			Expect(errors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "delete-test-2", Namespace: testNS}, cm)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should succeed when no ResourceTracker exists", func() {
			err := manager.DeleteManagedResources(ctx, plane)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Helper functions", func() {
		Describe("resourceKey", func() {
			It("should generate unique keys for resources", func() {
				r1 := &unstructured.Unstructured{}
				r1.SetAPIVersion("v1")
				r1.SetKind("ConfigMap")
				r1.SetNamespace("ns1")
				r1.SetName("cm1")

				r2 := &unstructured.Unstructured{}
				r2.SetAPIVersion("v1")
				r2.SetKind("ConfigMap")
				r2.SetNamespace("ns1")
				r2.SetName("cm2")

				r3 := &unstructured.Unstructured{}
				r3.SetAPIVersion("v1")
				r3.SetKind("Secret")
				r3.SetNamespace("ns1")
				r3.SetName("cm1")

				key1 := resourceKey(r1)
				key2 := resourceKey(r2)
				key3 := resourceKey(r3)

				Expect(key1).NotTo(Equal(key2)) // Different name
				Expect(key1).NotTo(Equal(key3)) // Different kind
			})
		})

		Describe("isManagedByPlane", func() {
			It("should return true for resources managed by the same plane", func() {
				existing := &unstructured.Unstructured{}
				existing.SetLabels(map[string]string{
					LabelPlaneName:      "test-plane",
					LabelPlaneNamespace: "test-ns",
					LabelManagedBy:      ManagedByClusterPlane,
				})

				desired := &unstructured.Unstructured{}
				desired.SetLabels(map[string]string{
					LabelPlaneName:      "test-plane",
					LabelPlaneNamespace: "test-ns",
					LabelManagedBy:      ManagedByClusterPlane,
				})

				Expect(isManagedByPlane(existing, desired)).To(BeTrue())
			})

			It("should return false for resources not managed by ClusterPlane", func() {
				existing := &unstructured.Unstructured{}
				existing.SetLabels(map[string]string{
					LabelManagedBy: "something-else",
				})

				desired := &unstructured.Unstructured{}
				desired.SetLabels(map[string]string{
					LabelPlaneName: "test-plane",
				})

				Expect(isManagedByPlane(existing, desired)).To(BeFalse())
			})

			It("should return false for resources managed by different plane", func() {
				existing := &unstructured.Unstructured{}
				existing.SetLabels(map[string]string{
					LabelPlaneName:      "plane-A",
					LabelPlaneNamespace: "ns",
					LabelManagedBy:      ManagedByClusterPlane,
				})

				desired := &unstructured.Unstructured{}
				desired.SetLabels(map[string]string{
					LabelPlaneName:      "plane-B",
					LabelPlaneNamespace: "ns",
					LabelManagedBy:      ManagedByClusterPlane,
				})

				Expect(isManagedByPlane(existing, desired)).To(BeFalse())
			})

			It("should return false when existing has no labels", func() {
				existing := &unstructured.Unstructured{}
				desired := &unstructured.Unstructured{}
				desired.SetLabels(map[string]string{
					LabelPlaneName: "test-plane",
				})

				Expect(isManagedByPlane(existing, desired)).To(BeFalse())
			})
		})
	})
})

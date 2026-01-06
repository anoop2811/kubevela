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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
)

var _ = Describe("ComponentRenderer", func() {
	var (
		ctx      context.Context
		renderer *CompositeRenderer
	)

	BeforeEach(func() {
		ctx = context.Background()
		renderer = NewCompositeRenderer()
	})

	Describe("RawComponentRenderer", func() {
		var rawRenderer *RawComponentRenderer

		BeforeEach(func() {
			rawRenderer = NewRawComponentRenderer()
		})

		Describe("SupportsType", func() {
			It("should support 'raw' type", func() {
				Expect(rawRenderer.SupportsType("raw")).To(BeTrue())
			})

			It("should support 'kubernetes' type", func() {
				Expect(rawRenderer.SupportsType("kubernetes")).To(BeTrue())
			})

			It("should not support other types", func() {
				Expect(rawRenderer.SupportsType("helm")).To(BeFalse())
				Expect(rawRenderer.SupportsType("kustomize")).To(BeFalse())
				Expect(rawRenderer.SupportsType("terraform")).To(BeFalse())
			})
		})

		Describe("Render", func() {
			var plane *v1alpha1.ClusterPlane

			BeforeEach(func() {
				plane = &v1alpha1.ClusterPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-plane",
						Namespace:  "test-namespace",
						Generation: 1,
					},
				}
			})

			It("should render a single ConfigMap resource", func() {
				configMapJSON := map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "my-config",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				}
				rawBytes, _ := json.Marshal(configMapJSON)

				component := &v1alpha1.PlaneComponent{
					Name: "my-configmap",
					Type: "raw",
					Properties: &runtime.RawExtension{
						Raw: rawBytes,
					},
				}

				resources, err := rawRenderer.Render(ctx, plane, component)
				Expect(err).NotTo(HaveOccurred())
				Expect(resources).To(HaveLen(1))

				cm := resources[0]
				Expect(cm.GetAPIVersion()).To(Equal("v1"))
				Expect(cm.GetKind()).To(Equal("ConfigMap"))
				Expect(cm.GetName()).To(Equal("my-config"))
				Expect(cm.GetNamespace()).To(Equal("test-namespace")) // Defaulted to plane namespace

				// Check labels
				labels := cm.GetLabels()
				Expect(labels[LabelPlaneName]).To(Equal("test-plane"))
				Expect(labels[LabelPlaneNamespace]).To(Equal("test-namespace"))
				Expect(labels[LabelPlaneComponent]).To(Equal("my-configmap"))
				Expect(labels[LabelManagedBy]).To(Equal(ManagedByClusterPlane))
			})

			It("should render an array of resources", func() {
				resourcesJSON := []interface{}{
					map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name": "config-1",
						},
					},
					map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name": "secret-1",
						},
					},
				}
				rawBytes, _ := json.Marshal(resourcesJSON)

				component := &v1alpha1.PlaneComponent{
					Name: "multi-resources",
					Type: "raw",
					Properties: &runtime.RawExtension{
						Raw: rawBytes,
					},
				}

				resources, err := rawRenderer.Render(ctx, plane, component)
				Expect(err).NotTo(HaveOccurred())
				Expect(resources).To(HaveLen(2))

				Expect(resources[0].GetKind()).To(Equal("ConfigMap"))
				Expect(resources[0].GetName()).To(Equal("config-1"))
				Expect(resources[1].GetKind()).To(Equal("Secret"))
				Expect(resources[1].GetName()).To(Equal("secret-1"))
			})

			It("should handle List type resources", func() {
				listJSON := map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "List",
					"items": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name": "cm-from-list",
							},
						},
					},
				}
				rawBytes, _ := json.Marshal(listJSON)

				component := &v1alpha1.PlaneComponent{
					Name: "list-component",
					Type: "raw",
					Properties: &runtime.RawExtension{
						Raw: rawBytes,
					},
				}

				resources, err := rawRenderer.Render(ctx, plane, component)
				Expect(err).NotTo(HaveOccurred())
				Expect(resources).To(HaveLen(1))
				Expect(resources[0].GetName()).To(Equal("cm-from-list"))
			})

			It("should auto-generate names if not specified", func() {
				configMapJSON := map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{},
				}
				rawBytes, _ := json.Marshal(configMapJSON)

				component := &v1alpha1.PlaneComponent{
					Name: "unnamed-resource",
					Type: "raw",
					Properties: &runtime.RawExtension{
						Raw: rawBytes,
					},
				}

				resources, err := rawRenderer.Render(ctx, plane, component)
				Expect(err).NotTo(HaveOccurred())
				Expect(resources).To(HaveLen(1))
				Expect(resources[0].GetName()).To(Equal("unnamed-resource")) // Uses component name
			})

			It("should preserve explicit namespace for namespaced resources", func() {
				deployJSON := map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "my-deploy",
						"namespace": "custom-ns",
					},
				}
				rawBytes, _ := json.Marshal(deployJSON)

				component := &v1alpha1.PlaneComponent{
					Name: "deploy-component",
					Type: "raw",
					Properties: &runtime.RawExtension{
						Raw: rawBytes,
					},
				}

				resources, err := rawRenderer.Render(ctx, plane, component)
				Expect(err).NotTo(HaveOccurred())
				Expect(resources).To(HaveLen(1))
				Expect(resources[0].GetNamespace()).To(Equal("custom-ns"))
			})

			It("should not set namespace for cluster-scoped resources", func() {
				crJSON := map[string]interface{}{
					"apiVersion": "rbac.authorization.k8s.io/v1",
					"kind":       "ClusterRole",
					"metadata": map[string]interface{}{
						"name": "my-cluster-role",
					},
				}
				rawBytes, _ := json.Marshal(crJSON)

				component := &v1alpha1.PlaneComponent{
					Name: "cluster-role-component",
					Type: "raw",
					Properties: &runtime.RawExtension{
						Raw: rawBytes,
					},
				}

				resources, err := rawRenderer.Render(ctx, plane, component)
				Expect(err).NotTo(HaveOccurred())
				Expect(resources).To(HaveLen(1))
				Expect(resources[0].GetNamespace()).To(BeEmpty())
			})

			It("should add revision label when plane has current revision", func() {
				plane.Status.CurrentRevision = &v1alpha1.RevisionReference{
					Name:    "test-plane-v1.0.0",
					Version: "v1.0.0",
				}

				configMapJSON := map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "my-config",
					},
				}
				rawBytes, _ := json.Marshal(configMapJSON)

				component := &v1alpha1.PlaneComponent{
					Name: "my-configmap",
					Type: "raw",
					Properties: &runtime.RawExtension{
						Raw: rawBytes,
					},
				}

				resources, err := rawRenderer.Render(ctx, plane, component)
				Expect(err).NotTo(HaveOccurred())
				Expect(resources).To(HaveLen(1))

				labels := resources[0].GetLabels()
				Expect(labels[LabelPlaneRevision]).To(Equal("test-plane-v1.0.0"))
			})

			It("should add generation annotation", func() {
				configMapJSON := map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "my-config",
					},
				}
				rawBytes, _ := json.Marshal(configMapJSON)

				component := &v1alpha1.PlaneComponent{
					Name: "my-configmap",
					Type: "raw",
					Properties: &runtime.RawExtension{
						Raw: rawBytes,
					},
				}

				resources, err := rawRenderer.Render(ctx, plane, component)
				Expect(err).NotTo(HaveOccurred())
				Expect(resources).To(HaveLen(1))

				annotations := resources[0].GetAnnotations()
				Expect(annotations[AnnotationPlaneGeneration]).To(Equal("1"))
			})

			Context("error cases", func() {
				It("should error when properties is nil", func() {
					component := &v1alpha1.PlaneComponent{
						Name:       "no-props",
						Type:       "raw",
						Properties: nil,
					}

					_, err := rawRenderer.Render(ctx, plane, component)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("has no properties"))
				})

				It("should error when properties is empty", func() {
					component := &v1alpha1.PlaneComponent{
						Name: "empty-props",
						Type: "raw",
						Properties: &runtime.RawExtension{
							Raw: []byte{},
						},
					}

					_, err := rawRenderer.Render(ctx, plane, component)
					Expect(err).To(HaveOccurred())
				})

				It("should error when apiVersion is missing", func() {
					invalidJSON := map[string]interface{}{
						"kind": "ConfigMap",
						"metadata": map[string]interface{}{
							"name": "my-config",
						},
					}
					rawBytes, _ := json.Marshal(invalidJSON)

					component := &v1alpha1.PlaneComponent{
						Name: "no-apiversion",
						Type: "raw",
						Properties: &runtime.RawExtension{
							Raw: rawBytes,
						},
					}

					_, err := rawRenderer.Render(ctx, plane, component)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("missing apiVersion"))
				})

				It("should error when kind is missing", func() {
					invalidJSON := map[string]interface{}{
						"apiVersion": "v1",
						"metadata": map[string]interface{}{
							"name": "my-resource",
						},
					}
					rawBytes, _ := json.Marshal(invalidJSON)

					component := &v1alpha1.PlaneComponent{
						Name: "no-kind",
						Type: "raw",
						Properties: &runtime.RawExtension{
							Raw: rawBytes,
						},
					}

					_, err := rawRenderer.Render(ctx, plane, component)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("missing kind"))
				})

				It("should error when JSON is invalid", func() {
					component := &v1alpha1.PlaneComponent{
						Name: "invalid-json",
						Type: "raw",
						Properties: &runtime.RawExtension{
							Raw: []byte("{invalid json}"),
						},
					}

					_, err := rawRenderer.Render(ctx, plane, component)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to parse"))
				})
			})
		})
	})

	Describe("CompositeRenderer", func() {
		var plane *v1alpha1.ClusterPlane

		BeforeEach(func() {
			plane = &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "composite-test",
					Namespace:  "default",
					Generation: 1,
				},
			}
		})

		It("should render raw type components", func() {
			configMapJSON := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "test-cm",
				},
			}
			rawBytes, _ := json.Marshal(configMapJSON)

			component := &v1alpha1.PlaneComponent{
				Name: "test-component",
				Type: "raw",
				Properties: &runtime.RawExtension{
					Raw: rawBytes,
				},
			}

			resources, err := renderer.Render(ctx, plane, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources).To(HaveLen(1))
		})

		It("should error for unsupported component types", func() {
			component := &v1alpha1.PlaneComponent{
				Name: "helm-component",
				Type: "helm",
				Properties: &runtime.RawExtension{
					Raw: []byte(`{"chart": "nginx"}`),
				},
			}

			_, err := renderer.Render(ctx, plane, component)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no renderer found"))
		})

		Describe("RenderAll", func() {
			It("should render all components and collect results", func() {
				cm1JSON, _ := json.Marshal(map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "cm1"},
				})
				cm2JSON, _ := json.Marshal(map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "cm2"},
				})

				plane.Spec.Components = []v1alpha1.PlaneComponent{
					{
						Name: "component-1",
						Type: "raw",
						Properties: &runtime.RawExtension{
							Raw: cm1JSON,
						},
					},
					{
						Name: "component-2",
						Type: "raw",
						Properties: &runtime.RawExtension{
							Raw: cm2JSON,
						},
					},
				}

				results := renderer.RenderAll(ctx, plane)
				Expect(results).To(HaveLen(2))
				Expect(results[0].ComponentName).To(Equal("component-1"))
				Expect(results[0].Error).NotTo(HaveOccurred())
				Expect(results[0].Resources).To(HaveLen(1))
				Expect(results[1].ComponentName).To(Equal("component-2"))
				Expect(results[1].Error).NotTo(HaveOccurred())
				Expect(results[1].Resources).To(HaveLen(1))
			})

			It("should continue rendering other components when one fails", func() {
				validJSON, _ := json.Marshal(map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "valid"},
				})

				plane.Spec.Components = []v1alpha1.PlaneComponent{
					{
						Name: "valid-component",
						Type: "raw",
						Properties: &runtime.RawExtension{
							Raw: validJSON,
						},
					},
					{
						Name:       "invalid-component",
						Type:       "raw",
						Properties: nil, // Will cause error
					},
					{
						Name: "another-valid",
						Type: "raw",
						Properties: &runtime.RawExtension{
							Raw: validJSON,
						},
					},
				}

				results := renderer.RenderAll(ctx, plane)
				Expect(results).To(HaveLen(3))

				// First should succeed
				Expect(results[0].Error).NotTo(HaveOccurred())
				Expect(results[0].Resources).To(HaveLen(1))

				// Second should fail
				Expect(results[1].Error).To(HaveOccurred())
				Expect(results[1].Resources).To(BeNil())

				// Third should succeed
				Expect(results[2].Error).NotTo(HaveOccurred())
				Expect(results[2].Resources).To(HaveLen(1))
			})
		})
	})

	Describe("FuncRenderer", func() {
		It("should support custom rendering functions", func() {
			customRenderer := NewFuncRenderer([]string{"custom-type"}, func(ctx context.Context, plane *v1alpha1.ClusterPlane, component *v1alpha1.PlaneComponent) ([]*unstructured.Unstructured, error) {
				return []*unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "custom.io/v1",
							"kind":       "CustomResource",
							"metadata": map[string]interface{}{
								"name": component.Name + "-custom",
							},
						},
					},
				}, nil
			})

			Expect(customRenderer.SupportsType("custom-type")).To(BeTrue())
			Expect(customRenderer.SupportsType("other")).To(BeFalse())

			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
			}
			component := &v1alpha1.PlaneComponent{
				Name: "test-component",
				Type: "custom-type",
			}

			resources, err := customRenderer.Render(ctx, plane, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(resources).To(HaveLen(1))
			Expect(resources[0].GetName()).To(Equal("test-component-custom"))
		})
	})

	Describe("Helper Functions", func() {
		Describe("ParseComponentProperties", func() {
			type TestProps struct {
				Key   string `json:"key"`
				Value int    `json:"value"`
			}

			It("should parse properties into struct", func() {
				propsJSON, _ := json.Marshal(TestProps{Key: "test", Value: 42})
				component := &v1alpha1.PlaneComponent{
					Properties: &runtime.RawExtension{
						Raw: propsJSON,
					},
				}

				var parsed TestProps
				err := ParseComponentProperties(component, &parsed)
				Expect(err).NotTo(HaveOccurred())
				Expect(parsed.Key).To(Equal("test"))
				Expect(parsed.Value).To(Equal(42))
			})

			It("should handle nil properties", func() {
				component := &v1alpha1.PlaneComponent{
					Properties: nil,
				}

				var parsed TestProps
				err := ParseComponentProperties(component, &parsed)
				Expect(err).NotTo(HaveOccurred())
				Expect(parsed.Key).To(BeEmpty())
				Expect(parsed.Value).To(Equal(0))
			})
		})

		Describe("ToRawExtension", func() {
			It("should convert object to RawExtension", func() {
				obj := map[string]string{"key": "value"}
				ext, err := ToRawExtension(obj)
				Expect(err).NotTo(HaveOccurred())
				Expect(ext.Raw).NotTo(BeEmpty())

				var parsed map[string]string
				err = json.Unmarshal(ext.Raw, &parsed)
				Expect(err).NotTo(HaveOccurred())
				Expect(parsed["key"]).To(Equal("value"))
			})
		})

		Describe("isNamespaced", func() {
			It("should identify cluster-scoped resources", func() {
				clusterScoped := []string{
					"Namespace",
					"Node",
					"ClusterRole",
					"ClusterRoleBinding",
					"CustomResourceDefinition",
					"PersistentVolume",
					"StorageClass",
				}

				for _, kind := range clusterScoped {
					u := &unstructured.Unstructured{}
					u.SetKind(kind)
					Expect(isNamespaced(u)).To(BeFalse(), "Expected %s to be cluster-scoped", kind)
				}
			})

			It("should identify namespaced resources", func() {
				namespaced := []string{
					"ConfigMap",
					"Secret",
					"Pod",
					"Deployment",
					"Service",
					"PersistentVolumeClaim",
				}

				for _, kind := range namespaced {
					u := &unstructured.Unstructured{}
					u.SetKind(kind)
					Expect(isNamespaced(u)).To(BeTrue(), "Expected %s to be namespaced", kind)
				}
			})
		})
	})
})

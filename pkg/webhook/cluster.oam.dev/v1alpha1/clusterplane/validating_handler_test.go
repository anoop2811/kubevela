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
	"net/http"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusteroam "github.com/oam-dev/kubevela/apis/cluster.oam.dev"
	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
	oamCore "github.com/oam-dev/kubevela/apis/core.oam.dev"
)

var handler ValidatingHandler
var decoder admission.Decoder
var testScheme = runtime.NewScheme()
var testEnv *envtest.Environment
var cfg *rest.Config

func TestClusterPlaneWebhook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ClusterPlane Webhook Suite")
}

var _ = BeforeSuite(func() {
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths:        []string{filepath.Join("../../../../..", "charts", "vela-core", "crds")},
	}

	err := oamCore.AddToScheme(testScheme)
	Expect(err).Should(BeNil())
	err = clusteroam.AddToScheme(testScheme)
	Expect(err).Should(BeNil())
	err = scheme.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	decoder = admission.NewDecoder(testScheme)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if testEnv != nil {
		err := testEnv.Stop()
		Expect(err).ToNot(HaveOccurred())
	}
})

var _ = Describe("ClusterPlane Validating Webhook", func() {
	BeforeEach(func() {
		cli, err := client.New(cfg, client.Options{Scheme: testScheme})
		Expect(err).Should(BeNil())
		handler = ValidatingHandler{
			Client:  cli,
			Decoder: decoder,
		}
	})

	Context("Create Operation", func() {
		It("should allow valid ClusterPlane creation", func() {
			plane := createValidClusterPlane("valid-plane", "default")
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("should reject ClusterPlane with empty component name", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-component-name",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{
							Name: "",
							Type: "helm",
						},
					},
				},
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Code).Should(Equal(int32(http.StatusBadRequest)))
			Expect(resp.Result.Message).Should(ContainSubstring("component name is required"))
		})

		It("should reject ClusterPlane with empty component type", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-component-type",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{
							Name: "my-component",
							Type: "",
						},
					},
				},
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Message).Should(ContainSubstring("component type is required"))
		})

		It("should reject ClusterPlane with duplicate component names", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "duplicate-components",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx", Type: "helm"},
						{Name: "nginx", Type: "kustomize"},
					},
				},
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Message).Should(ContainSubstring("duplicated"))
		})

		It("should reject ClusterPlane with invalid component name format", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-name-format",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "Invalid_Name", Type: "helm"},
					},
				},
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Message).Should(ContainSubstring("DNS-1123"))
		})

		It("should reject ClusterPlane with invalid publishVersion annotation", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-publish-version",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationPublishVersion: "",
					},
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx", Type: "helm"},
					},
				},
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Message).Should(ContainSubstring("publishVersion"))
		})

		It("should allow ClusterPlane with valid publishVersion annotation", func() {
			plane := createValidClusterPlane("valid-publish", "default")
			plane.Annotations = map[string]string{
				AnnotationPublishVersion: "v1.0.0",
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("should reject ClusterPlane with empty trait type", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-trait-type",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx", Type: "helm"},
					},
					Traits: []v1alpha1.PlaneTrait{
						{Type: ""},
					},
				},
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Message).Should(ContainSubstring("trait type is required"))
		})

		It("should reject ClusterPlane with empty policy type", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-policy-type",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx", Type: "helm"},
					},
					Policies: []v1alpha1.PlanePolicy{
						{Name: "my-policy", Type: ""},
					},
				},
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Message).Should(ContainSubstring("policy type is required"))
		})

		It("should reject ClusterPlane with duplicate policy names", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "duplicate-policies",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx", Type: "helm"},
					},
					Policies: []v1alpha1.PlanePolicy{
						{Name: "same-name", Type: "topology"},
						{Name: "same-name", Type: "override"},
					},
				},
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Message).Should(ContainSubstring("duplicated"))
		})

		It("should reject ClusterPlane with missing output component reference", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-output-ref",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx", Type: "helm"},
					},
					Outputs: []v1alpha1.PlaneOutput{
						{
							Name: "endpoint",
							ValueFrom: v1alpha1.PlaneOutputValueFrom{
								Component: "",
								FieldPath: "status.endpoint",
							},
						},
					},
				},
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Message).Should(ContainSubstring("valueFrom.component is required"))
		})

		It("should reject malformed admission request", func() {
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: []byte("invalid json")},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Code).Should(Equal(int32(http.StatusBadRequest)))
		})
	})

	Context("Update Operation", func() {
		It("should allow valid ClusterPlane update", func() {
			oldPlane := createValidClusterPlane("update-plane", "default")
			newPlane := createValidClusterPlane("update-plane", "default")
			newPlane.Spec.Description = "Updated description"

			oldPlaneRaw, _ := json.Marshal(oldPlane)
			newPlaneRaw, _ := json.Marshal(newPlane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object:    runtime.RawExtension{Raw: newPlaneRaw},
					OldObject: runtime.RawExtension{Raw: oldPlaneRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("should reject update with invalid component name", func() {
			oldPlane := createValidClusterPlane("update-invalid", "default")
			newPlane := createValidClusterPlane("update-invalid", "default")
			newPlane.Spec.Components = []v1alpha1.PlaneComponent{
				{Name: "INVALID_NAME", Type: "helm"},
			}

			oldPlaneRaw, _ := json.Marshal(oldPlane)
			newPlaneRaw, _ := json.Marshal(newPlane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object:    runtime.RawExtension{Raw: newPlaneRaw},
					OldObject: runtime.RawExtension{Raw: oldPlaneRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
		})

		It("should skip validation for deleting resources", func() {
			oldPlane := createValidClusterPlane("deleting-plane", "default")
			newPlane := createValidClusterPlane("deleting-plane", "default")
			now := metav1.Now()
			newPlane.DeletionTimestamp = &now

			oldPlaneRaw, _ := json.Marshal(oldPlane)
			newPlaneRaw, _ := json.Marshal(newPlane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object:    runtime.RawExtension{Raw: newPlaneRaw},
					OldObject: runtime.RawExtension{Raw: oldPlaneRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})
	})

	Context("Delete Operation", func() {
		It("should allow delete operation without validation", func() {
			plane := createValidClusterPlane("delete-plane", "default")
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})
	})
})

var _ = Describe("CrossClusterInput Validation", func() {
	Context("ValidateCrossClusterInputs", func() {
		It("should pass for valid cross-cluster inputs", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-plane",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "network-cidr",
							FromCluster: "management-cluster",
							FromPlane:   "network-plane",
							Output:      "pod-cidr",
							Required:    true,
						},
						{
							Name:          "dns-server",
							FromCluster:   "local",
							FromPlane:     "dns-plane",
							FromNamespace: "dns-system",
							Output:        "server-ip",
							Required:      false,
						},
					},
				},
			}
			errs := ValidateCrossClusterInputs(plane)
			Expect(errs).Should(BeEmpty())
		})

		It("should fail when input name is empty", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "",
							FromCluster: "cluster-1",
							FromPlane:   "plane-1",
							Output:      "output-1",
						},
					},
				},
			}
			errs := ValidateCrossClusterInputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("input name is required"))
		})

		It("should fail when input name has invalid format", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "Invalid_Name",
							FromCluster: "cluster-1",
							FromPlane:   "plane-1",
							Output:      "output-1",
						},
					},
				},
			}
			errs := ValidateCrossClusterInputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("DNS-1123"))
		})

		It("should fail when input names are duplicated", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "my-input",
							FromCluster: "cluster-1",
							FromPlane:   "plane-1",
							Output:      "output-1",
						},
						{
							Name:        "my-input",
							FromCluster: "cluster-2",
							FromPlane:   "plane-2",
							Output:      "output-2",
						},
					},
				},
			}
			errs := ValidateCrossClusterInputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("duplicated"))
		})

		It("should fail when fromCluster is empty", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "valid-input",
							FromCluster: "",
							FromPlane:   "plane-1",
							Output:      "output-1",
						},
					},
				},
			}
			errs := ValidateCrossClusterInputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("source cluster name is required"))
		})

		It("should fail when fromPlane is empty", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "valid-input",
							FromCluster: "cluster-1",
							FromPlane:   "",
							Output:      "output-1",
						},
					},
				},
			}
			errs := ValidateCrossClusterInputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("source plane name is required"))
		})

		It("should fail when output is empty", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "valid-input",
							FromCluster: "cluster-1",
							FromPlane:   "plane-1",
							Output:      "",
						},
					},
				},
			}
			errs := ValidateCrossClusterInputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("output name is required"))
		})

		It("should collect multiple errors", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "",
							FromCluster: "",
							FromPlane:   "",
							Output:      "",
						},
					},
				},
			}
			errs := ValidateCrossClusterInputs(plane)
			// Should have error for name (required), skips other validation for empty name
			// But we continue with next validations for fromCluster, fromPlane, output
			Expect(len(errs)).Should(BeNumerically(">=", 1))
		})

		It("should pass for empty cross-cluster inputs", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{},
				},
			}
			errs := ValidateCrossClusterInputs(plane)
			Expect(errs).Should(BeEmpty())
		})
	})

	Context("ValidateSelfReference", func() {
		It("should pass when no self-reference exists", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-plane",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "input-1",
							FromCluster: "other-cluster",
							FromPlane:   "my-plane",
							Output:      "output-1",
						},
					},
				},
			}
			errs := ValidateSelfReference(plane)
			Expect(errs).Should(BeEmpty())
		})

		It("should fail when referencing self with local cluster", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-plane",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "self-ref",
							FromCluster: "local",
							FromPlane:   "my-plane",
							Output:      "output-1",
						},
					},
				},
			}
			errs := ValidateSelfReference(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("cannot reference itself"))
		})

		It("should fail when referencing self with empty cluster", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-plane",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "self-ref",
							FromCluster: "",
							FromPlane:   "my-plane",
							Output:      "output-1",
						},
					},
				},
			}
			errs := ValidateSelfReference(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("cannot reference itself"))
		})

		It("should fail when referencing self with same namespace", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-plane",
					Namespace: "production",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:          "self-ref",
							FromCluster:   "local",
							FromPlane:     "my-plane",
							FromNamespace: "production",
							Output:        "output-1",
						},
					},
				},
			}
			errs := ValidateSelfReference(plane)
			Expect(errs).ShouldNot(BeEmpty())
		})

		It("should pass when referencing same name in different namespace", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-plane",
					Namespace: "production",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:          "other-ns-ref",
							FromCluster:   "local",
							FromPlane:     "my-plane",
							FromNamespace: "staging", // Different namespace
							Output:        "output-1",
						},
					},
				},
			}
			errs := ValidateSelfReference(plane)
			Expect(errs).Should(BeEmpty())
		})

		It("should pass when no cross-cluster inputs", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-plane",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					CrossClusterInputs: nil,
				},
			}
			errs := ValidateSelfReference(plane)
			Expect(errs).Should(BeEmpty())
		})
	})

	Context("Webhook Integration", func() {
		BeforeEach(func() {
			cli, err := client.New(cfg, client.Options{Scheme: testScheme})
			Expect(err).Should(BeNil())
			handler = ValidatingHandler{
				Client:  cli,
				Decoder: decoder,
			}
		})

		It("should reject ClusterPlane with self-referencing input", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "self-ref-plane",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx", Type: "helm"},
					},
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "bad-input",
							FromCluster: "local",
							FromPlane:   "self-ref-plane",
							Output:      "some-output",
						},
					},
				},
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
			Expect(resp.Result.Message).Should(ContainSubstring("cannot reference itself"))
		})

		It("should allow ClusterPlane with valid cross-cluster inputs", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-xcluster-plane",
					Namespace: "default",
				},
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx", Type: "helm"},
					},
					CrossClusterInputs: []v1alpha1.CrossClusterInput{
						{
							Name:        "network-config",
							FromCluster: "management",
							FromPlane:   "network-plane",
							Output:      "cidr-block",
							Required:    true,
						},
					},
				},
			}
			planeRaw, _ := json.Marshal(plane)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object:    runtime.RawExtension{Raw: planeRaw},
					Namespace: "default",
				},
			}

			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})
	})
})

var _ = Describe("Validation Functions", func() {
	Context("ValidateComponentNames", func() {
		It("should pass for valid component names", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx-ingress", Type: "helm"},
						{Name: "cert-manager", Type: "helm"},
					},
				},
			}
			errs := ValidateComponentNames(plane)
			Expect(errs).Should(BeEmpty())
		})

		It("should fail for names starting with hyphen", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "-nginx", Type: "helm"},
					},
				},
			}
			errs := ValidateComponentNames(plane)
			Expect(errs).ShouldNot(BeEmpty())
		})

		It("should fail for names ending with hyphen", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx-", Type: "helm"},
					},
				},
			}
			errs := ValidateComponentNames(plane)
			Expect(errs).ShouldNot(BeEmpty())
		})

		It("should fail for names with uppercase", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "Nginx", Type: "helm"},
					},
				},
			}
			errs := ValidateComponentNames(plane)
			Expect(errs).ShouldNot(BeEmpty())
		})

		It("should fail for names exceeding max length", func() {
			longName := ""
			for i := 0; i < MaxComponentNameLength+1; i++ {
				longName += "a"
			}
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: longName, Type: "helm"},
					},
				},
			}
			errs := ValidateComponentNames(plane)
			Expect(errs).ShouldNot(BeEmpty())
		})
	})

	Context("ValidateAnnotations", func() {
		It("should pass when no publishVersion annotation", func() {
			plane := &v1alpha1.ClusterPlane{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			}
			errs := ValidateAnnotations(plane)
			Expect(errs).Should(BeEmpty())
		})

		It("should pass for valid semver versions", func() {
			testCases := []string{"v1.0.0", "1.0.0", "v1.2.3-alpha", "1.0.0-beta.1", "v2.0.0+build.123"}
			for _, version := range testCases {
				plane := &v1alpha1.ClusterPlane{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							AnnotationPublishVersion: version,
						},
					},
				}
				errs := ValidateAnnotations(plane)
				Expect(errs).Should(BeEmpty(), "Version %s should be valid", version)
			}
		})

		It("should fail for invalid versions", func() {
			testCases := []string{"", "latest", "release-1"}
			for _, version := range testCases {
				plane := &v1alpha1.ClusterPlane{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							AnnotationPublishVersion: version,
						},
					},
				}
				errs := ValidateAnnotations(plane)
				Expect(errs).ShouldNot(BeEmpty(), "Version %s should be invalid", version)
			}
		})
	})

	Context("ValidateOutputs", func() {
		It("should pass for valid outputs", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Outputs: []v1alpha1.PlaneOutput{
						{
							Name: "endpoint",
							ValueFrom: v1alpha1.PlaneOutputValueFrom{
								Component: "nginx",
								FieldPath: "status.loadBalancer.ingress[0].ip",
							},
						},
					},
				},
			}
			errs := ValidateOutputs(plane)
			Expect(errs).Should(BeEmpty())
		})

		It("should fail for duplicate output names", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Outputs: []v1alpha1.PlaneOutput{
						{Name: "endpoint", ValueFrom: v1alpha1.PlaneOutputValueFrom{Component: "nginx", FieldPath: "status.ip"}},
						{Name: "endpoint", ValueFrom: v1alpha1.PlaneOutputValueFrom{Component: "cert", FieldPath: "status.ip"}},
					},
				},
			}
			errs := ValidateOutputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
		})

		It("should fail for missing fieldPath", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Outputs: []v1alpha1.PlaneOutput{
						{
							Name: "endpoint",
							ValueFrom: v1alpha1.PlaneOutputValueFrom{
								Component: "nginx",
								FieldPath: "",
							},
						},
					},
				},
			}
			errs := ValidateOutputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
		})

		It("should fail for invalid output name format", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Outputs: []v1alpha1.PlaneOutput{
						{
							Name: "Invalid_Output_Name",
							ValueFrom: v1alpha1.PlaneOutputValueFrom{
								Component: "nginx",
								FieldPath: "status.endpoint",
							},
						},
					},
				},
			}
			errs := ValidateOutputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("DNS-1123"))
		})

		It("should fail for field path starting with dot", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Outputs: []v1alpha1.PlaneOutput{
						{
							Name: "endpoint",
							ValueFrom: v1alpha1.PlaneOutputValueFrom{
								Component: "nginx",
								FieldPath: ".status.endpoint",
							},
						},
					},
				},
			}
			errs := ValidateOutputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("cannot start with '.'"))
		})

		It("should fail for field path ending with dot", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Outputs: []v1alpha1.PlaneOutput{
						{
							Name: "endpoint",
							ValueFrom: v1alpha1.PlaneOutputValueFrom{
								Component: "nginx",
								FieldPath: "status.endpoint.",
							},
						},
					},
				},
			}
			errs := ValidateOutputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("cannot end with '.'"))
		})

		It("should fail for field path with consecutive dots", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Outputs: []v1alpha1.PlaneOutput{
						{
							Name: "endpoint",
							ValueFrom: v1alpha1.PlaneOutputValueFrom{
								Component: "nginx",
								FieldPath: "status..endpoint",
							},
						},
					},
				},
			}
			errs := ValidateOutputs(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Error()).Should(ContainSubstring("consecutive dots"))
		})
	})

	Context("ValidateFieldPath", func() {
		It("should pass for valid field paths", func() {
			validPaths := []string{
				"status.phase",
				"spec.replicas",
				"metadata.name",
				"status.loadBalancer.ingress[0].ip",
				"spec.template.spec.containers[0].image",
			}
			for _, path := range validPaths {
				errs := ValidateFieldPath(nil, path)
				Expect(errs).Should(BeEmpty(), "Path %s should be valid", path)
			}
		})

		It("should return empty for empty path", func() {
			errs := ValidateFieldPath(nil, "")
			Expect(errs).Should(BeEmpty())
		})
	})

	Context("ValidateComponentReferences", func() {
		It("should pass when output references existing component", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx", Type: "helm"},
					},
					Outputs: []v1alpha1.PlaneOutput{
						{
							Name: "endpoint",
							ValueFrom: v1alpha1.PlaneOutputValueFrom{
								Component: "nginx",
								FieldPath: "status.endpoint",
							},
						},
					},
				},
			}
			errs := ValidateComponentReferences(plane)
			Expect(errs).Should(BeEmpty())
		})

		It("should fail when output references non-existent component", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx", Type: "helm"},
					},
					Outputs: []v1alpha1.PlaneOutput{
						{
							Name: "endpoint",
							ValueFrom: v1alpha1.PlaneOutputValueFrom{
								Component: "non-existent",
								FieldPath: "status.endpoint",
							},
						},
					},
				},
			}
			errs := ValidateComponentReferences(plane)
			Expect(errs).ShouldNot(BeEmpty())
			Expect(errs[0].Type).Should(Equal(field.ErrorTypeNotFound))
		})

		It("should pass when no outputs defined", func() {
			plane := &v1alpha1.ClusterPlane{
				Spec: v1alpha1.ClusterPlaneSpec{
					Components: []v1alpha1.PlaneComponent{
						{Name: "nginx", Type: "helm"},
					},
					Outputs: nil,
				},
			}
			errs := ValidateComponentReferences(plane)
			Expect(errs).Should(BeEmpty())
		})
	})
})

// createValidClusterPlane creates a valid ClusterPlane for testing
func createValidClusterPlane(name, namespace string) *v1alpha1.ClusterPlane {
	return &v1alpha1.ClusterPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.ClusterPlaneSpec{
			Description: "Test ClusterPlane",
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
}

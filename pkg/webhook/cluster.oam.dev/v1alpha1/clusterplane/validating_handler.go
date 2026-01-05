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
	"net/http"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/logging"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
)

var _ admission.Handler = &ValidatingHandler{}

// ValidatingHandler handles ClusterPlane validation
type ValidatingHandler struct {
	Client client.Client
	// Decoder decodes objects
	Decoder admission.Decoder
}

// Handle validates ClusterPlane spec
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	startTime := time.Now()
	ctx = logging.WithRequestID(ctx, string(req.UID))
	logger := logging.NewHandlerLogger(ctx, req, "ClusterPlaneValidator")

	// Track validation metrics
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.ClusterPlaneWebhookDuration.WithLabelValues(string(req.Operation)).Observe(duration)
		metrics.ClusterPlaneWebhookTotal.WithLabelValues(string(req.Operation)).Inc()
	}()

	logger.WithStep("start").Info("Starting admission validation for ClusterPlane resource",
		"operation", req.Operation,
		"clusterPlaneName", req.Name,
		"namespace", req.Namespace)

	// Decode the ClusterPlane
	plane := &v1alpha1.ClusterPlane{}
	if err := h.Decoder.Decode(req, plane); err != nil {
		logger.WithStep("decode").WithError(err).Error(err, "Unable to decode admission request payload into ClusterPlane object")
		metrics.ClusterPlaneWebhookErrors.WithLabelValues(string(req.Operation), "decode").Inc()
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode: %w (requestUID=%s)", err, req.UID))
	}

	if req.Namespace != "" {
		plane.Namespace = req.Namespace
	}

	logger = logger.WithValues(logging.FieldGeneration, plane.Generation)
	logger.WithStep("decode").Info("Successfully decoded ClusterPlane from admission request",
		"clusterPlaneName", plane.Name,
		"namespace", plane.Namespace,
		"componentCount", len(plane.Spec.Components))

	switch req.Operation {
	case admissionv1.Create:
		logger.WithStep("validate-create").Info("Validating ClusterPlane creation")
		if allErrs := h.ValidateCreate(ctx, plane); len(allErrs) > 0 {
			mergedErr := mergeErrors(allErrs)
			logger.WithStep("validate-create").WithError(mergedErr).Error(mergedErr,
				"ClusterPlane creation validation failed",
				"errorCount", len(allErrs),
				"clusterPlaneName", plane.Name)
			metrics.ClusterPlaneWebhookErrors.WithLabelValues(string(req.Operation), "validation").Inc()
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("%w (requestUID=%s)", mergedErr, req.UID))
		}
		logger.WithStep("validate-create").WithSuccess(true).Info("ClusterPlane creation validation completed successfully")

	case admissionv1.Update:
		logger.WithStep("validate-update").Info("Validating ClusterPlane update")
		oldPlane := &v1alpha1.ClusterPlane{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, oldPlane); err != nil {
			logger.WithStep("decode-old").WithError(err).Error(err, "Unable to decode previous ClusterPlane state")
			metrics.ClusterPlaneWebhookErrors.WithLabelValues(string(req.Operation), "decode_old").Inc()
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode old object: %w (requestUID=%s)", err, req.UID))
		}

		logger = logger.WithValues("oldGeneration", oldPlane.Generation)

		if plane.ObjectMeta.DeletionTimestamp.IsZero() {
			if allErrs := h.ValidateUpdate(ctx, plane, oldPlane); len(allErrs) > 0 {
				mergedErr := mergeErrors(allErrs)
				logger.WithStep("validate-update").WithError(mergedErr).Error(mergedErr,
					"ClusterPlane update validation failed",
					"errorCount", len(allErrs),
					"clusterPlaneName", plane.Name)
				metrics.ClusterPlaneWebhookErrors.WithLabelValues(string(req.Operation), "validation").Inc()
				return admission.Errored(http.StatusBadRequest, fmt.Errorf("%w (requestUID=%s)", mergedErr, req.UID))
			}
			logger.WithStep("validate-update").WithSuccess(true).Info("ClusterPlane update validation completed successfully",
				"generationChange", fmt.Sprintf("%d->%d", oldPlane.Generation, plane.Generation))
		} else {
			logger.WithStep("skip-validation").Info("Skipping ClusterPlane validation - resource is being deleted")
		}

	case admissionv1.Delete:
		logger.WithStep("skip-validation").Info("Skipping ClusterPlane validation - DELETE operations do not require validation")

	default:
		logger.WithStep("skip-validation").Info("Skipping ClusterPlane validation - unsupported operation type",
			"operation", req.Operation)
	}

	logger.WithStep("complete").WithSuccess(true, startTime).Info("ClusterPlane admission validation completed successfully",
		"clusterPlaneName", req.Name,
		"operation", req.Operation,
		"namespace", req.Namespace)

	return admission.ValidationResponse(true, "")
}

// mergeErrors combines multiple field errors into a single error message
func mergeErrors(errs field.ErrorList) error {
	s := ""
	for _, err := range errs {
		s += fmt.Sprintf("field \"%s\": %s error encountered, %s. ", err.Field, err.Type, err.Detail)
	}
	return fmt.Errorf(s)
}

// RegisterValidatingHandler registers the ClusterPlane validating webhook
func RegisterValidatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/validating-cluster-oam-dev-v1alpha1-clusterplanes", &webhook.Admission{Handler: &ValidatingHandler{
		Client:  mgr.GetClient(),
		Decoder: admission.NewDecoder(mgr.GetScheme()),
	}})
}

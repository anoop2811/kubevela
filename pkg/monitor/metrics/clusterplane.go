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

package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	velametrics "github.com/kubevela/pkg/monitor/metrics"
)

var (
	// ClusterPlaneReconcileDuration reports the duration of ClusterPlane reconciliation
	ClusterPlaneReconcileDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "kubevela_clusterplane_reconcile_duration_seconds",
		Help:    "ClusterPlane reconcile duration in seconds.",
		Buckets: velametrics.FineGrainedBuckets,
	}, []string{"namespace", "name"})

	// ClusterPlaneReconcileTotal reports the total number of ClusterPlane reconciliations
	ClusterPlaneReconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kubevela_clusterplane_reconcile_total",
		Help: "Total number of ClusterPlane reconciliations.",
	}, []string{"namespace", "name"})

	// ClusterPlaneReconcileErrors reports the number of ClusterPlane reconciliation errors
	ClusterPlaneReconcileErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kubevela_clusterplane_reconcile_errors_total",
		Help: "Total number of ClusterPlane reconciliation errors.",
	}, []string{"namespace", "name", "error_type"})

	// ClusterPlanePhase reports the current phase of each ClusterPlane
	// Values: 0=Draft, 1=Publishing, 2=Running, 3=Degraded, 4=Suspended, 5=Failed, -1=Unknown
	ClusterPlanePhase = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_phase",
		Help: "ClusterPlane phase as numeric value (0=Draft, 1=Publishing, 2=Running, 3=Degraded, 4=Suspended, 5=Failed, -1=Unknown).",
	}, []string{"namespace", "name"})

	// ClusterPlaneComponentCount reports the number of components in each ClusterPlane
	ClusterPlaneComponentCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_component_count",
		Help: "Number of components defined in a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlaneRevisionCount reports the number of revisions for each ClusterPlane
	ClusterPlaneRevisionCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_revision_count",
		Help: "Number of revisions for a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlaneInfo provides info labels for each ClusterPlane (always 1)
	ClusterPlaneInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_info",
		Help: "Information about ClusterPlane resources. Always 1, labels provide metadata.",
	}, []string{"namespace", "name", "current_revision"})

	// ClusterPlaneWebhookDuration reports the duration of webhook validation
	ClusterPlaneWebhookDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "kubevela_clusterplane_webhook_duration_seconds",
		Help:    "ClusterPlane webhook validation duration in seconds.",
		Buckets: velametrics.FineGrainedBuckets,
	}, []string{"operation"})

	// ClusterPlaneWebhookTotal reports the total number of webhook validations
	ClusterPlaneWebhookTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kubevela_clusterplane_webhook_total",
		Help: "Total number of ClusterPlane webhook validations.",
	}, []string{"operation"})

	// ClusterPlaneWebhookErrors reports the number of webhook validation errors
	ClusterPlaneWebhookErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kubevela_clusterplane_webhook_errors_total",
		Help: "Total number of ClusterPlane webhook validation errors.",
	}, []string{"operation", "error_type"})

	// ClusterPlaneHealthyComponents reports the number of healthy components
	ClusterPlaneHealthyComponents = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_healthy_components",
		Help: "Number of healthy components in a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlaneDegradedComponents reports the number of degraded components
	ClusterPlaneDegradedComponents = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_degraded_components",
		Help: "Number of degraded components in a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlaneFailedComponents reports the number of failed components
	ClusterPlaneFailedComponents = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_failed_components",
		Help: "Number of failed components in a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlanePendingComponents reports the number of pending components
	ClusterPlanePendingComponents = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_pending_components",
		Help: "Number of pending components in a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlaneOverallHealthy reports whether the plane is overall healthy (1) or not (0)
	ClusterPlaneOverallHealthy = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_overall_healthy",
		Help: "Whether the ClusterPlane is overall healthy (1) or not (0).",
	}, []string{"namespace", "name"})

	// ClusterPlaneInputResolutionDuration reports the duration of input resolution
	ClusterPlaneInputResolutionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "kubevela_clusterplane_input_resolution_duration_seconds",
		Help:    "ClusterPlane cross-cluster input resolution duration in seconds.",
		Buckets: velametrics.FineGrainedBuckets,
	}, []string{"namespace", "name"})

	// ClusterPlaneInputsTotal reports the total number of cross-cluster inputs
	ClusterPlaneInputsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_inputs_total",
		Help: "Total number of cross-cluster inputs for a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlaneInputsResolved reports the number of resolved inputs
	ClusterPlaneInputsResolved = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_inputs_resolved",
		Help: "Number of resolved cross-cluster inputs for a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlaneInputsFailed reports the number of failed input resolutions
	ClusterPlaneInputsFailed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_inputs_failed",
		Help: "Number of failed cross-cluster input resolutions for a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlaneInputResolutionErrors reports errors during input resolution
	ClusterPlaneInputResolutionErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kubevela_clusterplane_input_resolution_errors_total",
		Help: "Total number of cross-cluster input resolution errors.",
	}, []string{"namespace", "name", "input_name", "source_cluster"})

	// ClusterPlaneOutputResolutionDuration reports the duration of output resolution
	ClusterPlaneOutputResolutionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "kubevela_clusterplane_output_resolution_duration_seconds",
		Help:    "ClusterPlane output resolution duration in seconds.",
		Buckets: velametrics.FineGrainedBuckets,
	}, []string{"namespace", "name"})

	// ClusterPlaneOutputsTotal reports the total number of outputs
	ClusterPlaneOutputsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_outputs_total",
		Help: "Total number of outputs for a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlaneOutputsResolved reports the number of resolved outputs
	ClusterPlaneOutputsResolved = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_outputs_resolved",
		Help: "Number of resolved outputs for a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlaneOutputsFailed reports the number of failed output resolutions
	ClusterPlaneOutputsFailed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubevela_clusterplane_outputs_failed",
		Help: "Number of failed output resolutions for a ClusterPlane.",
	}, []string{"namespace", "name"})

	// ClusterPlaneOutputResolutionErrors reports errors during output resolution
	ClusterPlaneOutputResolutionErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kubevela_clusterplane_output_resolution_errors_total",
		Help: "Total number of output resolution errors.",
	}, []string{"namespace", "name", "output_name", "component"})
)

var registerClusterPlaneMetricsOnce sync.Once

// RegisterClusterPlaneMetrics registers ClusterPlane metrics with the controller-runtime metrics registry
func RegisterClusterPlaneMetrics() {
	registerClusterPlaneMetricsOnce.Do(func() {
		metrics.Registry.MustRegister(
			ClusterPlaneReconcileDuration,
			ClusterPlaneReconcileTotal,
			ClusterPlaneReconcileErrors,
			ClusterPlanePhase,
			ClusterPlaneComponentCount,
			ClusterPlaneRevisionCount,
			ClusterPlaneInfo,
			ClusterPlaneWebhookDuration,
			ClusterPlaneWebhookTotal,
			ClusterPlaneWebhookErrors,
			ClusterPlaneHealthyComponents,
			ClusterPlaneDegradedComponents,
			ClusterPlaneFailedComponents,
			ClusterPlanePendingComponents,
			ClusterPlaneOverallHealthy,
			ClusterPlaneInputResolutionDuration,
			ClusterPlaneInputsTotal,
			ClusterPlaneInputsResolved,
			ClusterPlaneInputsFailed,
			ClusterPlaneInputResolutionErrors,
			ClusterPlaneOutputResolutionDuration,
			ClusterPlaneOutputsTotal,
			ClusterPlaneOutputsResolved,
			ClusterPlaneOutputsFailed,
			ClusterPlaneOutputResolutionErrors,
		)
	})
}

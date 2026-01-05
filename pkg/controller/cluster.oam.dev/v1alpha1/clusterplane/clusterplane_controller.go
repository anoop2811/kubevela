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

	"github.com/crossplane/crossplane-runtime/pkg/event"
	ctrlrec "github.com/kubevela/pkg/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/oam-dev/kubevela/apis/cluster.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
)

const (
	// AnnotationPublishVersion is the annotation key for publishing a plane revision
	AnnotationPublishVersion = "plane.oam.dev/publishVersion"

	// ConditionTypeReconciled indicates whether the plane has been successfully reconciled
	ConditionTypeReconciled condition.ConditionType = "Reconciled"

	// ConditionTypeHealthy indicates whether the plane is healthy
	ConditionTypeHealthy condition.ConditionType = "Healthy"
)

// Reconciler reconciles a ClusterPlane object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder event.Recorder
	options
}

type options struct {
	concurrentReconciles int
	revisionLimit        int
}

// Reconcile is the main logic for ClusterPlane controller
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := ctrlrec.NewReconcileContext(ctx)
	defer cancel()

	startTime := time.Now()
	klog.InfoS("Reconcile ClusterPlane", "clusterPlane", klog.KRef(req.Namespace, req.Name))

	// Track reconciliation metrics
	defer func() {
		metrics.ClusterPlaneReconcileDuration.WithLabelValues(req.Namespace, req.Name).Observe(time.Since(startTime).Seconds())
		metrics.ClusterPlaneReconcileTotal.WithLabelValues(req.Namespace, req.Name).Inc()
	}()

	// Fetch the ClusterPlane instance
	var plane v1alpha1.ClusterPlane
	if err := r.Get(ctx, req.NamespacedName, &plane); err != nil {
		// ClusterPlane was deleted, clean up metrics
		metrics.ClusterPlanePhase.DeleteLabelValues(req.Namespace, req.Name)
		metrics.ClusterPlaneComponentCount.DeleteLabelValues(req.Namespace, req.Name)
		metrics.ClusterPlaneRevisionCount.DeleteLabelValues(req.Namespace, req.Name)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Update observedGeneration
	if plane.Status.ObservedGeneration != plane.Generation {
		plane.Status.ObservedGeneration = plane.Generation
	}

	// Set current time for status updates
	now := metav1.Now()
	plane.Status.LastUpdated = &now

	// Handle revision creation if publishVersion is set
	if _, hasPublish := plane.Annotations[AnnotationPublishVersion]; hasPublish {
		if err := r.reconcileRevision(ctx, &plane); err != nil {
			klog.ErrorS(err, "Failed to reconcile revision",
				"clusterPlane", klog.KRef(req.Namespace, req.Name))
			metrics.ClusterPlaneReconcileErrors.WithLabelValues(req.Namespace, req.Name, "revision").Inc()

			// Set failed phase and condition
			plane.Status.Phase = v1alpha1.PlanePhaseFailed
			plane.SetConditions(condition.Condition{
				Type:               ConditionTypeReconciled,
				Status:             "False",
				LastTransitionTime: now,
				Reason:             "RevisionError",
				Message:            err.Error(),
			})

			// Update status even on error to reflect failure
			if statusErr := r.UpdateStatus(ctx, &plane); statusErr != nil {
				klog.ErrorS(statusErr, "Failed to update status after revision error")
			}
			return ctrl.Result{}, err
		}
	}

	// Determine phase based on annotations and current state
	phase := r.determinePhase(&plane)
	previousPhase := plane.Status.Phase
	plane.Status.Phase = phase

	// Update phase metric
	metrics.ClusterPlanePhase.WithLabelValues(req.Namespace, req.Name).Set(phaseToMetricValue(phase))
	metrics.ClusterPlaneComponentCount.WithLabelValues(req.Namespace, req.Name).Set(float64(len(plane.Spec.Components)))

	// Set conditions based on phase
	switch phase {
	case v1alpha1.PlanePhaseDraft:
		plane.SetConditions(condition.Condition{
			Type:               ConditionTypeReconciled,
			Status:             "True",
			LastTransitionTime: now,
			Reason:             "Draft",
			Message:            "ClusterPlane is in draft mode. Add plane.oam.dev/publishVersion annotation to publish.",
		})
	case v1alpha1.PlanePhasePublishing:
		plane.SetConditions(condition.Condition{
			Type:               ConditionTypeReconciled,
			Status:             "True",
			LastTransitionTime: now,
			Reason:             "Publishing",
			Message:            "ClusterPlane revision is being created.",
		})
	case v1alpha1.PlanePhaseRunning:
		plane.SetConditions(condition.Condition{
			Type:               ConditionTypeReconciled,
			Status:             "True",
			LastTransitionTime: now,
			Reason:             "Running",
			Message:            "ClusterPlane is running.",
		})
		plane.SetConditions(condition.Condition{
			Type:               ConditionTypeHealthy,
			Status:             "True",
			LastTransitionTime: now,
			Reason:             "AllComponentsHealthy",
			Message:            "All components are healthy.",
		})
	case v1alpha1.PlanePhaseFailed:
		plane.SetConditions(condition.Condition{
			Type:               ConditionTypeReconciled,
			Status:             "False",
			LastTransitionTime: now,
			Reason:             "ReconcileError",
			Message:            "Failed to reconcile ClusterPlane.",
		})
	}

	// Log phase transition
	if previousPhase != phase {
		klog.InfoS("ClusterPlane phase transition",
			"clusterPlane", klog.KRef(req.Namespace, req.Name),
			"previousPhase", previousPhase,
			"newPhase", phase)
		r.Recorder.Event(&plane, event.Normal("PhaseTransition",
			"Phase changed from %s to %s", string(previousPhase), string(phase)))
	}

	// Update status
	if err := r.UpdateStatus(ctx, &plane); err != nil {
		klog.ErrorS(err, "Failed to update ClusterPlane status",
			"clusterPlane", klog.KRef(req.Namespace, req.Name))
		metrics.ClusterPlaneReconcileErrors.WithLabelValues(req.Namespace, req.Name, "status_update").Inc()
		return ctrl.Result{}, err
	}

	klog.InfoS("Successfully reconciled ClusterPlane",
		"clusterPlane", klog.KRef(req.Namespace, req.Name),
		"phase", phase,
		"components", len(plane.Spec.Components),
		"currentRevision", plane.Status.CurrentRevision,
		"duration", time.Since(startTime))

	return ctrl.Result{}, nil
}

// reconcileRevision handles the creation of ClusterPlaneRevision
func (r *Reconciler) reconcileRevision(ctx context.Context, plane *v1alpha1.ClusterPlane) error {
	// Use the revision management logic
	revision, isNew, err := ReconcilePlaneRevision(ctx, r.Client, r.Recorder, plane, r.revisionLimit)
	if err != nil {
		return err
	}

	// Update status with current revision reference
	if revision != nil {
		plane.Status.CurrentRevision = GetRevisionReference(revision)
		plane.Status.RevisionCount++

		if isNew {
			klog.InfoS("Created new ClusterPlaneRevision",
				"clusterPlane", klog.KRef(plane.Namespace, plane.Name),
				"revision", revision.Name,
				"version", revision.Spec.PlaneSnapshot.Version)
		}
	}

	return nil
}

// determinePhase determines the phase of the ClusterPlane based on its current state
func (r *Reconciler) determinePhase(plane *v1alpha1.ClusterPlane) v1alpha1.PlanePhase {
	// Check for publishVersion annotation
	if _, hasPublish := plane.Annotations[AnnotationPublishVersion]; !hasPublish {
		return v1alpha1.PlanePhaseDraft
	}

	// If we have a current revision, we're running
	if plane.Status.CurrentRevision != nil && plane.Status.CurrentRevision.Name != "" {
		return v1alpha1.PlanePhaseRunning
	}

	// If publishVersion is set but no revision yet, we're publishing
	return v1alpha1.PlanePhasePublishing
}

// UpdateStatus updates ClusterPlane's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, plane *v1alpha1.ClusterPlane, opts ...client.SubResourceUpdateOption) error {
	status := plane.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, client.ObjectKey{Namespace: plane.Namespace, Name: plane.Name}, plane); err != nil {
			return
		}
		plane.Status = status
		return r.Status().Update(ctx, plane, opts...)
	})
}

// SetupWithManager sets up the controller with the Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = event.NewAPIRecorder(mgr.GetEventRecorderFor("ClusterPlane")).
		WithAnnotations("controller", "ClusterPlane")
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&v1alpha1.ClusterPlane{}).
		Complete(r)
}

// Setup adds a controller that reconciles ClusterPlane
func Setup(mgr ctrl.Manager, args oamctrl.Args) error {
	// Register ClusterPlane metrics
	metrics.RegisterClusterPlaneMetrics()

	r := Reconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		options: parseOptions(args),
	}
	return r.SetupWithManager(mgr)
}

func parseOptions(args oamctrl.Args) options {
	revLimit := args.DefRevisionLimit
	if revLimit == 0 {
		revLimit = 10 // Default revision limit for ClusterPlane
	}
	return options{
		concurrentReconciles: args.ConcurrentReconciles,
		revisionLimit:        revLimit,
	}
}

// phaseToMetricValue converts a PlanePhase to a numeric value for metrics
func phaseToMetricValue(phase v1alpha1.PlanePhase) float64 {
	switch phase {
	case v1alpha1.PlanePhaseDraft:
		return 0
	case v1alpha1.PlanePhasePublishing:
		return 1
	case v1alpha1.PlanePhaseRunning:
		return 2
	case v1alpha1.PlanePhaseSuspended:
		return 3
	case v1alpha1.PlanePhaseFailed:
		return 4
	default:
		return -1
	}
}

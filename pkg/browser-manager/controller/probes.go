package controller

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

// readinessProbeForBrowser marks the pod ready once x11vnc listens, so the console
// doesn't open the viewer before VNC is up.
// Every field the API server defaults is set, so the probe compares equal after a round trip.
func readinessProbeForBrowser() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(5900)},
		},
		InitialDelaySeconds: 2,
		PeriodSeconds:       2,
		TimeoutSeconds:      1,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
}

// livenessProbeForBrowser restarts the pod when the action server isn't answering,
// for example when it failed to start or Chromium disconnected.
func livenessProbeForBrowser() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/healthz",
				Port:   intstr.FromInt32(3000),
				Scheme: corev1.URISchemeHTTP,
			},
		},
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		TimeoutSeconds:      2,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
}

// probesMatch reports whether the browser container already has the desired probes.
func probesMatch(container *corev1.Container) bool {
	return equality.Semantic.DeepEqual(container.ReadinessProbe, readinessProbeForBrowser()) &&
		equality.Semantic.DeepEqual(container.LivenessProbe, livenessProbeForBrowser())
}

// handleDeploymentProbesMismatch adds or restores the probes on Deployments created
// before they existed, or edited since.
func (r *BrowserReconciler) handleDeploymentProbesMismatch(
	ctx context.Context,
	log *logr.Logger,
	deployment *appsv1.Deployment,
) (ctrl.Result, error) {
	deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = readinessProbeForBrowser()
	deployment.Spec.Template.Spec.Containers[0].LivenessProbe = livenessProbeForBrowser()

	if err := r.Update(ctx, deployment); err != nil {
		log.Error(err, "Failed to update Deployment probes",
			"Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

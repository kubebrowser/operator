package controller

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

// healthzForBrowser checks the action server. It listens only once Chromium is up,
// a few seconds after x11vnc attaches to Xvfb, and answers 503 once Chromium disconnects.
func healthzForBrowser() corev1.ProbeHandler {
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path:   "/healthz",
			Port:   intstr.FromInt32(3000),
			Scheme: corev1.URISchemeHTTP,
		},
	}
}

// readinessProbeForBrowser marks the pod ready once the browser answers, so the console
// doesn't open the viewer before VNC and the action API are up. It probes HTTP rather
// than the VNC port, because x11vnc logs every probe connection as an error.
func readinessProbeForBrowser() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:        healthzForBrowser(),
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
		ProbeHandler:        healthzForBrowser(),
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		TimeoutSeconds:      2,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}
}

// strategyForBrowser replaces the pod instead of rolling it: a browser is a single
// stateful replica, and a rolling update needs room for a second browser pod, which
// leaves the new pod Pending on a full node.
func strategyForBrowser() appsv1.DeploymentStrategy {
	return appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
}

// rolloutSettingsMatch reports whether the Deployment already has the desired
// strategy and probes. It compares only the fields set here, so a field the API
// server starts defaulting later can't make it report a mismatch on every reconcile.
func rolloutSettingsMatch(deployment *appsv1.Deployment) bool {
	container := &deployment.Spec.Template.Spec.Containers[0]
	return deployment.Spec.Strategy.Type == strategyForBrowser().Type &&
		probeMatches(container.ReadinessProbe, readinessProbeForBrowser()) &&
		probeMatches(container.LivenessProbe, livenessProbeForBrowser())
}

// probeMatches compares the HTTP check and timings of a probe built by this package.
func probeMatches(actual, desired *corev1.Probe) bool {
	if actual == nil || actual.HTTPGet == nil {
		return false
	}
	return actual.HTTPGet.Path == desired.HTTPGet.Path &&
		actual.HTTPGet.Port == desired.HTTPGet.Port &&
		actual.HTTPGet.Scheme == desired.HTTPGet.Scheme &&
		actual.InitialDelaySeconds == desired.InitialDelaySeconds &&
		actual.PeriodSeconds == desired.PeriodSeconds &&
		actual.TimeoutSeconds == desired.TimeoutSeconds &&
		actual.SuccessThreshold == desired.SuccessThreshold &&
		actual.FailureThreshold == desired.FailureThreshold
}

// handleDeploymentRolloutSettingsMismatch sets the strategy and probes on Deployments
// created before they existed, or edited since.
func (r *BrowserReconciler) handleDeploymentRolloutSettingsMismatch(
	ctx context.Context,
	log *logr.Logger,
	deployment *appsv1.Deployment,
) (ctrl.Result, error) {
	deployment.Spec.Strategy = strategyForBrowser()
	deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = readinessProbeForBrowser()
	deployment.Spec.Template.Spec.Containers[0].LivenessProbe = livenessProbeForBrowser()

	if err := r.Update(ctx, deployment); err != nil {
		log.Error(err, "Failed to update Deployment strategy and probes",
			"Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

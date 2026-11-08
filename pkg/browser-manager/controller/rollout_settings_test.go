package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Browser rollout settings", func() {
	deploymentWith := func(strategy appsv1.DeploymentStrategy, readiness, liveness *corev1.Probe) *appsv1.Deployment {
		return &appsv1.Deployment{Spec: appsv1.DeploymentSpec{
			Strategy: strategy,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{ReadinessProbe: readiness, LivenessProbe: liveness}},
			}},
		}}
	}

	It("ignores fields the API server fills in that the operator doesn't set", func() {
		readiness := readinessProbeForBrowser()
		readiness.TerminationGracePeriodSeconds = new(int64)
		readiness.HTTPGet.Host = "defaulted.example"
		readiness.HTTPGet.HTTPHeaders = []corev1.HTTPHeader{{Name: "X-Defaulted", Value: "1"}}
		liveness := livenessProbeForBrowser()
		liveness.TerminationGracePeriodSeconds = new(int64)

		Expect(rolloutSettingsMatch(deploymentWith(strategyForBrowser(), readiness, liveness))).To(BeTrue())
	})

	It("reports a mismatch when a field the operator sets differs", func() {
		Expect(rolloutSettingsMatch(deploymentWith(
			appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
			readinessProbeForBrowser(), livenessProbeForBrowser()))).To(BeFalse())

		Expect(rolloutSettingsMatch(deploymentWith(strategyForBrowser(), nil, livenessProbeForBrowser()))).To(BeFalse())

		tcp := readinessProbeForBrowser()
		tcp.HTTPGet = nil
		tcp.TCPSocket = &corev1.TCPSocketAction{Port: intstr.FromInt32(5900)}
		Expect(rolloutSettingsMatch(deploymentWith(strategyForBrowser(), tcp, livenessProbeForBrowser()))).To(BeFalse())

		slower := livenessProbeForBrowser()
		slower.PeriodSeconds = 20
		Expect(rolloutSettingsMatch(deploymentWith(strategyForBrowser(), readinessProbeForBrowser(), slower))).To(BeFalse())
	})
})

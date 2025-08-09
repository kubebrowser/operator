package controller

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/kubebrowser/operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *BrowserSystemReconciler) reconcileBrowserController(
	ctx context.Context,
	system *corev1alpha1.BrowserSystem,
	log *logr.Logger,
) (ReconcileResult, error) {
	// Check if the browsercontroller deployment already exists, if not create a new one
	browserControllerDeployment := &appsv1.Deployment{}
	err := r.Get(ctx,
		types.NamespacedName{Name: getBrowserControllerName(system.Name), Namespace: system.Namespace},
		browserControllerDeployment,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err := r.createDeployment(ctx, system, log, ForController)
			if err != nil {
				return ReconciledError, err
			}
			return ReconciledUpdated, nil
		} else {
			log.Error(err, "Failed to get deployment", "for", ForController)
			return ReconciledError, err
		}
	}

	return ReconciledOk, nil
}

func (r *BrowserSystemReconciler) getBrowserControllerDeployment(
	system *corev1alpha1.BrowserSystem) (*appsv1.Deployment, error) {
	ls := getLabelsForSystem(system.Name)

	browserControllerImage, err := getBrowserControllerImage()
	if err != nil {
		return nil, err
	}

	browserImage, err := getBrowserImage()
	if err != nil {
		return nil, err
	}

	terminationGracePeriod := int64(10)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getBrowserControllerName(system.Name),
			Namespace: system.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &terminationGracePeriod,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					ServiceAccountName: r.getServiceAccountName(system),
					Containers: []corev1.Container{{
						Image:           browserControllerImage,
						Args:            []string{"--leader-elect", "--health-probe-bind-address=:8081"},
						Name:            "browser-controller",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Env:             []corev1.EnvVar{{Name: "BROWSER_IMAGE", Value: browserImage}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("200Mi"),
							},
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/healthz",
									Port: intstr.FromInt(8081),
								},
							},
							InitialDelaySeconds: 15,
							PeriodSeconds:       20,
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/readyz",
									Port: intstr.FromInt(8081),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
						},
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             ptr.To(true),
							AllowPrivilegeEscalation: ptr.To(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
					},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(system, dep, r.Scheme); err != nil {
		return nil, err
	}
	return dep, nil
}

func (r *BrowserSystemReconciler) deleteAnyBrowsers(ctx context.Context, log *logr.Logger) error {
	log.Info("Deleting all browsers")
	browserList := corev1alpha1.BrowserList{}
	err := r.List(ctx, &browserList, &client.ListOptions{})
	if err != nil {
		return err
	}

	for _, browser := range browserList.Items {
		err := r.Delete(ctx, &browser, &client.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete browser named %s in namespace %s: %s", browser.Name, browser.Namespace, err)
		}
	}

	return nil
}

func (r *BrowserSystemReconciler) getServiceAccountName(_ *corev1alpha1.BrowserSystem) string {
	return os.Getenv("SERVICE_ACCOUNT_NAME")
}

func getBrowserControllerImage() (string, error) {
	var envVar = "BROWSER_MANAGER_IMAGE"
	image, exists := os.LookupEnv(envVar)
	if !exists {
		return "", fmt.Errorf("unable to find %s environment variable with the image", envVar)
	}

	return image, nil
}

func getBrowserImage() (string, error) {
	var envVar = "BROWSER_IMAGE"
	image, exists := os.LookupEnv(envVar)
	if !exists {
		return "", fmt.Errorf("unable to find %s environment variable with the image", envVar)
	}

	return image, nil
}

// takes system name and returns browser controller name
func getBrowserControllerName(systemName string) string {
	return systemName + "-browser-controller"
}

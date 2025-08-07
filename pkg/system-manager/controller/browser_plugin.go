package controller

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/kubebrowser/operator/api/v1alpha1"
	ocpv1 "github.com/openshift/api/console/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *BrowserSystemReconciler) createConsolePluginDeployment(ctx context.Context, system *corev1alpha1.BrowserSystem, log *logr.Logger) (ctrl.Result, error) {
	// Define a new deployment
	dep, err := r.getConsolePluginDeployment(system)
	if err != nil {
		log.Error(err, "14 Failed to define new Deployment resource for System Console Plugin")

		// The following implementation will update the status
		meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
			Status: metav1.ConditionFalse, Reason: "Reconciling",
			Message: fmt.Sprintf("Failed to create Service for the browser system (%s): (%s)", system.Name, err)})

		if err := r.Status().Update(ctx, system); err != nil {
			log.Error(err, "15 Failed to update System status 5")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	log.Info("Creating a new Deployment",
		"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
	if err = r.Create(ctx, dep); err != nil {
		log.Error(err, "16 Failed to create new Deployment",
			"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil

}

func (r *BrowserSystemReconciler) deleteConsolePluginDeployment(ctx context.Context, system *corev1alpha1.BrowserSystem, deployment *appsv1.Deployment, log *logr.Logger) (ctrl.Result, error) {
	log.Info("Deleting Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
	if err := r.Delete(ctx, deployment); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to delete deployment for console plugin")

			// The following implementation will update the status
			meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to delete Deployment for the console plugin (%s): (%s)", deployment.Name, err)})

			if err := r.Status().Update(ctx, system); err != nil {
				log.Error(err, "15 Failed to update System status 5")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
	}

	// deletion success move forward
	return ctrl.Result{}, nil
}

func (r *BrowserSystemReconciler) createConsolePluginService(ctx context.Context, system *corev1alpha1.BrowserSystem, log *logr.Logger) (ctrl.Result, error) {
	service, err := r.getConsolePluginService(system)
	if err != nil {
		log.Error(err, "14 Failed to define new Service resource for Console Plugin")

		// The following implementation will update the status
		meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
			Status: metav1.ConditionFalse, Reason: "Reconciling",
			Message: fmt.Sprintf("Failed to create Console Plugin Service for the browser system (%s): (%s)", system.Name, err)})

		if err := r.Status().Update(ctx, system); err != nil {
			log.Error(err, "15 Failed to update System status 5")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	log.Info("Creating a new service", "service.Name", service.Name)

	if err = r.Create(ctx, service); err != nil {
		log.Error(err, "16 Failed to create new service", "service.Name", service.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *BrowserSystemReconciler) deleteConsolePluginService(ctx context.Context, system *corev1alpha1.BrowserSystem, service *corev1.Service, log *logr.Logger) (ctrl.Result, error) {
	log.Info("Deleting Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
	if err := r.Delete(ctx, service); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to delete service for console plugin")

			// The following implementation will update the status
			meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to delete Service for the console plugin (%s): (%s)", service.Name, err)})

			if err := r.Status().Update(ctx, system); err != nil {
				log.Error(err, "15 Failed to update System status 5")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
	}

	// deletion success move forward
	return ctrl.Result{}, nil
}

func (r *BrowserSystemReconciler) createConsolePluginDefinition(ctx context.Context, system *corev1alpha1.BrowserSystem, log *logr.Logger) (ctrl.Result, error) {
	plugin, err := r.getConsolePluginDefinition(system)
	if err != nil {
		log.Error(err, "14 Failed to define new console plugin resource")

		// The following implementation will update the status
		meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
			Status: metav1.ConditionFalse, Reason: "Reconciling",
			Message: fmt.Sprintf("Failed to create Console Plugin Resource for the browser system (%s): (%s)", system.Name, err)})

		if err := r.Status().Update(ctx, system); err != nil {
			log.Error(err, "15 Failed to update System status 5")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	log.Info("Creating a new console plugin", "Name", plugin.Name)

	if err = r.Create(ctx, plugin); err != nil {
		log.Error(err, "16 Failed to create new console plugin", "Name", plugin.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *BrowserSystemReconciler) deleteConsolePluginDefinition(ctx context.Context, system *corev1alpha1.BrowserSystem, plugin *ocpv1.ConsolePlugin, log *logr.Logger) (ctrl.Result, error) {
	log.Info("Deleting Console Plugin", "Name", plugin.Name)
	if err := r.Delete(ctx, plugin); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to delete console plugin")

			// The following implementation will update the status
			meta.SetStatusCondition(&system.Status.Conditions, metav1.Condition{Type: typeAvailable,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to delete console plugin (%s): (%s)", plugin.Name, err)})

			if err := r.Status().Update(ctx, system); err != nil {
				log.Error(err, "15 Failed to update System status 5")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
	}
	// deletion success move forward
	return ctrl.Result{}, nil
}

func (r *BrowserSystemReconciler) getConsolePluginDeployment(
	system *corev1alpha1.BrowserSystem) (*appsv1.Deployment, error) {

	ls := getLabelsForSystem(system.Name)

	name, err := getConsolePluginName()
	if err != nil {
		return nil, err
	}

	image, err := getConsolePluginImage()
	if err != nil {
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: system.Namespace,
			Labels:    ls,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:           image,
						Name:            name,
						ImagePullPolicy: corev1.PullIfNotPresent,
						VolumeMounts:    []corev1.VolumeMount{{Name: "certs", MountPath: "/var/certs", ReadOnly: true}},
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
					Volumes: []corev1.Volume{
						{Name: "certs", VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: getConsolePluginCertsName(name),
							}}},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(system, deployment, r.Scheme); err != nil {
		return nil, err
	}
	return deployment, nil
}

func (r *BrowserSystemReconciler) getConsolePluginService(
	system *corev1alpha1.BrowserSystem) (*corev1.Service, error) {
	ls := getLabelsForSystem(system.Name)

	name, err := getConsolePluginName()
	if err != nil {
		return nil, err
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   system.Namespace,
			Labels:      ls,
			Annotations: map[string]string{"service.beta.openshift.io/serving-cert-secret-name": getConsolePluginCertsName(name)},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": name,
			},
			Ports: []corev1.ServicePort{{Port: 9443, TargetPort: intstr.FromInt(9443), Protocol: "TCP"}},
		},
	}
	if err := ctrl.SetControllerReference(system, service, r.Scheme); err != nil {
		return nil, err
	}
	return service, nil
}

func (r *BrowserSystemReconciler) getConsolePluginDefinition(
	system *corev1alpha1.BrowserSystem) (*ocpv1.ConsolePlugin, error) {
	ls := getLabelsForSystem(system.Name)

	name, err := getConsolePluginName()
	if err != nil {
		return nil, err
	}

	plugin := &ocpv1.ConsolePlugin{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: ls,
		},
		Spec: ocpv1.ConsolePluginSpec{
			Backend: ocpv1.ConsolePluginBackend{
				Type: "Service",
				Service: &ocpv1.ConsolePluginService{
					BasePath:  "/",
					Name:      name,
					Namespace: system.Namespace,
					Port:      9443,
				},
			},
			DisplayName: name,
			I18n: ocpv1.ConsolePluginI18n{
				LoadType: ocpv1.Preload,
			},
		},
	}

	return plugin, nil
}

func (r *BrowserSystemReconciler) deleteAnyConsolePlugin(ctx context.Context, log *logr.Logger) error {
	plugin := &ocpv1.ConsolePlugin{}
	name, err := getConsolePluginName()
	if err != nil {
		return err
	}

	err = r.Get(ctx, types.NamespacedName{Name: name}, plugin)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}

	log.Info("Deleting Console Plugin")

	err = r.Delete(ctx, plugin)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}

	// patch console to remove?

	return nil
}

func getConsolePluginImage() (string, error) {
	var envVar = "CONSOLE_PLUGIN_IMAGE"
	image, exists := os.LookupEnv(envVar)
	if !exists {
		return "", fmt.Errorf("unable to find %s environment variable with the image", envVar)
	}

	return image, nil
}

func getConsolePluginName() (string, error) {
	var envVar = "CONSOLE_PLUGIN_NAME"
	name, exists := os.LookupEnv(envVar)
	if !exists {
		return "", fmt.Errorf("unable to find %s environment variable with the image", envVar)
	}

	return name, nil
}

func getConsolePluginCertsName(consolePluginName string) string {
	return consolePluginName + "-certs"
}

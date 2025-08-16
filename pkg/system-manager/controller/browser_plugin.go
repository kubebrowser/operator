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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *BrowserSystemReconciler) reconcileBrowserPlugin(
	ctx context.Context,
	system *corev1alpha1.BrowserSystem,
	log *logr.Logger,
) (ReconcileResult, error) {

	consolePluginName := getConsolePluginName()
	/* console-plugin Deployment	*/
	pluginDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: consolePluginName, Namespace: system.Namespace}, pluginDeployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if shouldEnableConsolePlugin(system.Spec.EnableConsolePlugin) {
				// deployment not found && should be present => create new
				err := r.createDeployment(ctx, system, log, ForPlugin)
				if err != nil {
					return ReconciledError, err
				}
				return ReconciledUpdated, nil
			}
		} else {
			log.Error(err, "Failed to get deployment", "For", ForPlugin)
			return ReconciledError, err
		}
	} else if !shouldEnableConsolePlugin(system.Spec.EnableConsolePlugin) {
		// deployment found but shouldn't be present
		err := r.deleteResource(ctx, system, pluginDeployment, log)
		if err != nil {
			return ReconciledError, err
		}
		return ReconciledUpdated, nil
	} else {
		// deployment is here rightfully
		desiredPluginImage, err := getConsolePluginImage()
		if err != nil {
			return ReconciledError, err
		}
		if pluginDeployment.Spec.Template.Spec.Containers[0].Image != desiredPluginImage {
			pluginDeployment.Spec.Template.Spec.Containers[0].Image = desiredPluginImage
			if err := r.Update(ctx, pluginDeployment); err != nil {
				log.Error(err, "failed to update browser plugin deployment image")
				return ReconciledError, err
			}
			return ReconciledUpdated, nil
		}
	}
	/* console-plugin Deployment	*/

	/* console-plugin Service	*/
	pluginService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: consolePluginName, Namespace: system.Namespace}, pluginService)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if shouldEnableConsolePlugin(system.Spec.EnableConsolePlugin) {
				// service not found && should be present => create new
				err := r.createService(ctx, system, log, ForPlugin)
				if err != nil {
					return ReconciledError, err
				}
				return ReconciledUpdated, nil
			}
		} else {
			log.Error(err, "failed to get service", "for", ForPlugin)
			return ReconciledError, err
		}
	} else if !shouldEnableConsolePlugin(system.Spec.EnableConsolePlugin) {
		// service found but shouldn't be present
		err := r.deleteResource(ctx, system, pluginService, log)
		if err != nil {
			return ReconciledError, err
		}
		return ReconciledUpdated, nil
	}
	/* console-plugin Service	*/

	/* console-plugin Definition	*/
	consolePlugin := &ocpv1.ConsolePlugin{}
	err = r.Get(ctx, types.NamespacedName{Name: consolePluginName}, consolePlugin)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if shouldEnableConsolePlugin(system.Spec.EnableConsolePlugin) {
				// consoleplugin not found && should be present => create new
				err := r.createConsolePluginDefinition(ctx, system, log)
				if err != nil {
					return ReconciledError, err
				}
				return ReconciledUpdated, nil
			}
		} else {
			log.Error(err, "Failed to get consoleplugin")
			return ReconciledError, err
		}
	} else if !shouldEnableConsolePlugin(system.Spec.EnableConsolePlugin) {
		// service found but shouldn't be present
		err := r.deleteResource(ctx, system, consolePlugin, log)
		if err != nil {
			return ReconciledError, err
		}
		return ReconciledUpdated, nil
	}

	return ReconciledOk, nil
}

func (r *BrowserSystemReconciler) createConsolePluginDefinition(
	ctx context.Context,
	system *corev1alpha1.BrowserSystem,
	log *logr.Logger,
) error {
	plugin := r.getConsolePluginDefinition(system)
	log.Info("Creating a new console plugin", "Name", plugin.Name)
	if err := r.Create(ctx, plugin); err != nil {
		log.Error(err, "16 Failed to create new console plugin", "Name", plugin.Name)
		return err
	}

	return nil
}

func (r *BrowserSystemReconciler) getConsolePluginDeployment(
	system *corev1alpha1.BrowserSystem) (*appsv1.Deployment, error) {

	ls := getLabelsForSystem(system.Name)

	name := getConsolePluginName()

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

	name := getConsolePluginName()

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: system.Namespace,
			Labels:    ls,
			Annotations: map[string]string{
				"service.beta.openshift.io/serving-cert-secret-name": getConsolePluginCertsName(name),
			},
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
	system *corev1alpha1.BrowserSystem) *ocpv1.ConsolePlugin {
	ls := getLabelsForSystem(system.Name)

	name := getConsolePluginName()

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

	return plugin
}

func (r *BrowserSystemReconciler) deleteAnyConsolePlugin(ctx context.Context, log *logr.Logger) error {
	plugin := &ocpv1.ConsolePlugin{}
	name := getConsolePluginName()

	err := r.Get(ctx, types.NamespacedName{Name: name}, plugin)
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

func getConsolePluginName() string {
	var envVar = "CONSOLE_PLUGIN_NAME"
	name, exists := os.LookupEnv(envVar)
	if !exists {
		return "kubebrowser-plugin"
	}

	return name
}

func getConsolePluginCertsName(consolePluginName string) string {
	return consolePluginName + "-certs"
}

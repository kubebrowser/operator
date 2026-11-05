package controller

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/kubebrowser/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// browser-api only accepts requests proxied by the kube-apiserver, which it
// recognizes by the front-proxy client certificate. The CA and allowed names
// live in a kube-system ConfigMap that browser-api can't mount, so the operator
// copies them into the browser-api namespace.
const (
	authVolumeName         = "auth"
	authMountPath          = "/auth"
	clientCAKey            = "requestheader-client-ca-file"
	allowedNamesKey        = "requestheader-allowed-names"
	frontProxyCAResync     = 10 * time.Minute
	extensionAuthNamespace = "kube-system"
	extensionAuthName      = "extension-apiserver-authentication"
)

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;create;update;delete

func getBrowserAPIAuthName(browserApiName string) string {
	return browserApiName + "-auth"
}

// The ConfigMaps are read without the cache so the operator doesn't have to
// list and watch every ConfigMap in the cluster.
func (r *BrowserSystemReconciler) uncachedReader() client.Reader {
	if r.APIReader != nil {
		return r.APIReader
	}
	return r.Client
}

func (r *BrowserSystemReconciler) reconcileBrowserApiAuth(
	ctx context.Context,
	system *corev1alpha1.BrowserSystem,
	log *logr.Logger,
) (ReconcileResult, error) {
	existing := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: getBrowserAPIAuthName(getBrowserAPIName(system.Name)), Namespace: system.Namespace}
	err := r.uncachedReader().Get(ctx, key, existing)
	found := err == nil
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "failed to get browser api auth configmap")
		return ReconciledError, err
	}

	if !shouldEnableAPIService(system.Spec.EnableApiService) {
		if found {
			if err := r.deleteResource(ctx, system, existing, log); err != nil {
				return ReconciledError, err
			}
			return ReconciledUpdated, nil
		}
		return ReconciledOk, nil
	}

	source := &corev1.ConfigMap{}
	err = r.uncachedReader().Get(ctx, types.NamespacedName{Name: extensionAuthName, Namespace: extensionAuthNamespace}, source)
	if err != nil {
		log.Error(err, "failed to get front-proxy CA", "configmap", extensionAuthNamespace+"/"+extensionAuthName)
		return ReconciledError, err
	}
	if source.Data[clientCAKey] == "" {
		err := fmt.Errorf("%s/%s has no %s; the kube-apiserver has no front-proxy CA configured",
			extensionAuthNamespace, extensionAuthName, clientCAKey)
		log.Error(err, "cannot secure browser api")
		return ReconciledError, err
	}
	data := map[string]string{
		clientCAKey:     source.Data[clientCAKey],
		allowedNamesKey: source.Data[allowedNamesKey],
	}

	if !found {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      key.Name,
				Namespace: key.Namespace,
				Labels:    getLabelsForSystem(system.Name),
			},
			Data: data,
		}
		if err := ctrl.SetControllerReference(system, configMap, r.Scheme); err != nil {
			return ReconciledError, err
		}
		log.Info("Creating browser api auth configmap", "ConfigMap.Name", configMap.Name)
		if err := r.Create(ctx, configMap); err != nil {
			log.Error(err, "failed to create browser api auth configmap")
			return ReconciledError, err
		}
		return ReconciledUpdated, nil
	}

	if !maps.Equal(existing.Data, data) {
		log.Info("Updating browser api auth configmap", "ConfigMap.Name", existing.Name)
		existing.Data = data
		if err := r.Update(ctx, existing); err != nil {
			log.Error(err, "failed to update browser api auth configmap")
			return ReconciledError, err
		}
		return ReconciledUpdated, nil
	}

	return ReconciledOk, nil
}

func hasVolume(podSpec corev1.PodSpec, name string) bool {
	for _, volume := range podSpec.Volumes {
		if volume.Name == name {
			return true
		}
	}
	return false
}

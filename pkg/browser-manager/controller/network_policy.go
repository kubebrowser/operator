package controller

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/kubebrowser/operator/api/v1alpha1"
	"github.com/kubebrowser/operator/pkg/browser-manager/utils"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// Browser pods serve VNC and the action API without authentication. RBAC is
// enforced by the kube-apiserver in front of browser-api, so only browser-api
// may connect to them; anything else could take over the session directly.
func networkPolicyForBrowser(browser *corev1alpha1.Browser) (*networkingv1.NetworkPolicy, error) {
	apiNamespace, apiName := os.Getenv("BROWSER_API_NAMESPACE"), os.Getenv("BROWSER_API_NAME")
	if apiNamespace == "" || apiName == "" {
		return nil, fmt.Errorf("BROWSER_API_NAMESPACE and BROWSER_API_NAME environment variables must be set")
	}

	tcp := corev1.ProtocolTCP
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      browser.Name,
			Namespace: browser.Namespace,
			Labels:    labelsForBrowser(browser.Name),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": browser.Name, "component": "browser"},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"kubernetes.io/metadata.name": apiNamespace},
					},
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": apiName},
					},
				}},
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &tcp, Port: ptrIntOrString(utils.BrowserPortNumber)},
					{Protocol: &tcp, Port: ptrIntOrString(utils.VNCPortNumber)},
				},
			}},
		},
	}, nil
}

func ptrIntOrString(port int) *intstr.IntOrString {
	value := intstr.FromInt(port)
	return &value
}

// reconcileNetworkPolicy creates or corrects the browser's NetworkPolicy and
// reports whether it changed anything.
func (r *BrowserReconciler) reconcileNetworkPolicy(
	ctx context.Context,
	browser *corev1alpha1.Browser,
	log *logr.Logger,
) (bool, error) {
	desired, err := networkPolicyForBrowser(browser)
	if err != nil {
		log.Error(err, "Failed to define NetworkPolicy for Browser")
		return false, err
	}
	if err := ctrl.SetControllerReference(browser, desired, r.Scheme); err != nil {
		return false, err
	}

	existing := &networkingv1.NetworkPolicy{}
	err = r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		log.Info("Creating a new NetworkPolicy",
			"NetworkPolicy.Namespace", desired.Namespace, "NetworkPolicy.Name", desired.Name)
		if err := r.Create(ctx, desired); err != nil {
			log.Error(err, "Failed to create NetworkPolicy")
			return false, err
		}
		return true, nil
	}
	if err != nil {
		log.Error(err, "Failed to get NetworkPolicy")
		return false, err
	}

	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		log.Info("Updating NetworkPolicy",
			"NetworkPolicy.Namespace", existing.Namespace, "NetworkPolicy.Name", existing.Name)
		existing.Spec = desired.Spec
		if err := r.Update(ctx, existing); err != nil {
			log.Error(err, "Failed to update NetworkPolicy")
			return false, err
		}
		return true, nil
	}

	return false, nil
}

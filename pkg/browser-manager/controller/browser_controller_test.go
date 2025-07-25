/*
Copyright 2025.

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

package controller

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/kubebrowser/operator/api/v1alpha1"
)

var _ = Describe("Browser controller", func() {
	Context("Browser controller test", func() {

		const BrowserName = "test-browser"

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      BrowserName,
				Namespace: BrowserName,
			},
		}

		typeNamespacedName := types.NamespacedName{
			Name:      BrowserName,
			Namespace: BrowserName,
		}
		browser := &corev1alpha1.Browser{}

		SetDefaultEventuallyTimeout(2 * time.Minute)
		SetDefaultEventuallyPollingInterval(time.Second)

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())

			By("Setting the Image ENV VAR which stores the Operand image")
			err = os.Setenv("BROWSER_IMAGE", "example.com/image:test")
			Expect(err).NotTo(HaveOccurred())

			By("creating the custom resource for the Kind Browser")
			err = k8sClient.Get(ctx, typeNamespacedName, browser)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				browser := &corev1alpha1.Browser{
					ObjectMeta: metav1.ObjectMeta{
						Name:      BrowserName,
						Namespace: namespace.Name,
					},
					Spec: corev1alpha1.BrowserSpec{
						Started: true,
						BrowserResources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("50m"),
								corev1.ResourceMemory: resource.MustParse("50Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("60m"),
								corev1.ResourceMemory: resource.MustParse("60Mi"),
							},
						},
					},
				}

				err = k8sClient.Create(ctx, browser)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind Browser")
			found := &corev1alpha1.Browser{}
			err := k8sClient.Get(ctx, typeNamespacedName, found)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Delete(context.TODO(), found)).To(Succeed())
			}).Should(Succeed())

			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations.
			// More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)

			By("Removing the Image ENV VAR which stores the Operand image")
			_ = os.Unsetenv("BROWSER_IMAGE")
		})

		It("should successfully reconcile a custom resource for Browser", func() {
			By("Checking if the custom resource was successfully created")
			Eventually(func(g Gomega) {
				found := &corev1alpha1.Browser{}
				Expect(k8sClient.Get(ctx, typeNamespacedName, found)).To(Succeed())
			}).Should(Succeed())

			By("Reconciling the custom resource created")
			browserReconciler := &BrowserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := browserReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func(g Gomega) {
				found := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, found)).To(Succeed())
			}).Should(Succeed())

			By("Reconciling the custom resource again")
			_, err = browserReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the latest Status Condition added to the Browser instance")
			Expect(k8sClient.Get(ctx, typeNamespacedName, browser)).To(Succeed())
			var conditions []metav1.Condition
			Expect(browser.Status.Conditions).To(ContainElement(
				HaveField("Type", Equal(typeAvailableBrowser)), &conditions))
			Expect(conditions).To(HaveLen(1), "Multiple conditions of type %s", typeAvailableBrowser)
			Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue), "condition %s", typeAvailableBrowser)
			Expect(conditions[0].Reason).To(Equal("Reconciling"), "condition %s", typeAvailableBrowser)
		})
	})
})

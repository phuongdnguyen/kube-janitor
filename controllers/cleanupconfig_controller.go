/*
Copyright 2022.

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

package controllers

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/utils/strings/slices"

	devv1alpha1 "github.com/phuongnd96/kube-janitor/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// CleanUpConfigReconciler reconciles a CleanUpConfig object
type CleanUpConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dev.dev,resources=cleanupconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dev.dev,resources=cleanupconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dev.dev,resources=cleanupconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CleanUpConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *CleanUpConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	defer log.Info("end reconcile")
	var err error
	conf := getConf()
	fmt.Printf("conf: %v\n", conf)
	foundCR := &devv1alpha1.CleanUpConfig{}
	err = r.Get(ctx, req.NamespacedName, foundCR)
	if err != nil {
		if errors.IsNotFound(err) {
			// Return and don't requeue
			log.Info("restore not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get CleanUpConfig")
		return ctrl.Result{}, err
	}
	cronCommand := fmt.Sprintf("kubectl annotate --overwrite cleanupconfig %v last-run-timestamp=\"$(date)\"", foundCR.Name)
	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      foundCR.Name,
			Namespace: foundCR.Namespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          foundCR.Spec.Schedule,
			ConcurrencyPolicy: batchv1.ReplaceConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							RestartPolicy: "OnFailure",
							Containers: []v1.Container{
								{
									Name:    foundCR.Name,
									Image:   "bitnami/kubectl:1.24.3-debian-11-r11",
									Command: []string{"sh", "-c", cronCommand},
								},
							},
							ServiceAccountName: foundCR.Name,
						},
					},
				},
			},
		},
	}
	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: cronjob.Name}, &batchv1.CronJob{})
	if err != nil && errors.IsNotFound(err) {
		ctrl.SetControllerReference(foundCR, cronjob, r.Scheme)
		if err = r.Create(ctx, cronjob, &client.CreateOptions{}); err != nil {
			log.Error(err, "create cronjob")
		}
	}
	serviceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      foundCR.Name,
			Namespace: req.Namespace,
		},
	}
	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: serviceAccount.Name}, &v1.ServiceAccount{})
	if err != nil && errors.IsNotFound(err) {
		ctrl.SetControllerReference(foundCR, serviceAccount, r.Scheme)
		if err = r.Create(ctx, serviceAccount, &client.CreateOptions{}); err != nil {
			log.Error(err, "create ServiceAccount")
		}
	}
	var policyRules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{"dev.dev"},
			Resources: []string{"cleanupconfigs"},
			Verbs:     []string{"get", "watch", "list", "patch", "update"},
		},
	}
	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      foundCR.Name,
			Namespace: req.Namespace,
		},
		Rules: policyRules,
	}
	if err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: role.Name}, &rbacv1.Role{}); err != nil && errors.IsNotFound(err) {
		ctrl.SetControllerReference(foundCR, role, r.Scheme)
		if err = r.Create(ctx, role, &client.CreateOptions{}); err != nil {
			log.Error(err, "create role")
		}
	}
	roleBinding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      foundCR.Name,
			Namespace: req.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      foundCR.Name,
			Namespace: req.Namespace,
		}},
	}
	if err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: roleBinding.Name}, &rbacv1.RoleBinding{}); err != nil && errors.IsNotFound(err) {
		ctrl.SetControllerReference(foundCR, roleBinding, r.Scheme)
		if err = r.Create(ctx, roleBinding, &client.CreateOptions{}); err != nil {
			log.Error(err, "create rolebinding")
		}
	}
	namespaceList := &v1.NamespaceList{}
	if err = r.List(ctx, namespaceList, &client.ListOptions{}); err != nil {
		log.Error(err, "list namespace")
	}
	for _, ns := range namespaceList.Items {
		log.Info("checking condition", "ns", ns.Name)
		// Dont process excluded namespaces
		if len(foundCR.Spec.ExcludedNamespaces) > 0 {
			if slices.Contains(foundCR.Spec.ExcludedNamespaces, ns.Name) {
				log.Info("skipping excluded namespace", ns.Name)
				continue
			}
		}
		// Dont process namespace with excludedLabels
		if len(foundCR.Spec.ExcludedLabels) > 0 {
			for _, label := range foundCR.Spec.ExcludedLabels {
				if !compareMap(ns.Labels, label) {
					log.Info("skipping namespace with excluded label", "ns.Name", ns.Name, "label", label)
					continue
				}
			}
		}

		// Process
		log.Info("processing", "ns", ns.Name)
		podList := &v1.PodList{}
		if err = r.List(ctx, podList, &client.ListOptions{
			Namespace: ns.Name,
		}); err != nil {
			log.Error(err, "list pod", "namespace", ns.Name)
		}
		defaultWhiteListedStatus := []string{"Running"}
		for _, pod := range podList.Items {
			if !slices.Contains(append(foundCR.Spec.WhitelistedStatus, defaultWhiteListedStatus...), string(pod.Status.Phase)) {
				log.Info("Deleting pod", "pod.Name", pod.Name)
				r.Delete(ctx, &pod, &client.DeleteOptions{})
			}
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CleanUpConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&devv1alpha1.CleanUpConfig{}).
		Complete(r)
}

func compareMap(a map[string]string, b map[string]string) bool {
	for k, v := range a {
		if b[k] == v {
			return true
		}
	}
	return false
}

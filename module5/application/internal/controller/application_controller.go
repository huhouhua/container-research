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
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	applicationv1 "github/huhouhua/container-research/application/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=application.aiops.com,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=application.aiops.com,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=application.aiops.com,resources=applications/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Application object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var app applicationv1.Application
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		logger.Error(err, "unable to fetch Application")
		return ctrl.Result{}, nil
	}
	labels := map[string]string{
		"app": app.Name,
	}
	logger.Info("Reconcile Application", "app", app.Name)

	if ret, err := r.createOrUpdateDeployment(ctx, app, labels); err != nil {
		return ret, err
	}
	if ret, err := r.createOrUpdateService(ctx, app, labels); err != nil {
		return ret, err
	}
	if ret, err := r.createOrUpdateIngress(ctx, app, labels); err != nil {
		return ret, err
	}
	// Update the status of the Application
	if err := r.Status().Update(ctx, &app); err != nil {
		logger.Error(err, "unable to update Application status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// Create or update Deployment
func (r *ApplicationReconciler) createOrUpdateDeployment(ctx context.Context, app applicationv1.Application, labels map[string]string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		replicas := int32(1)
		if app.Spec.Deployment.Replicas != 0 {
			app.Spec.Deployment.Replicas = replicas
		}
		deployment.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  app.Name,
						Image: app.Spec.Deployment.Image,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: app.Spec.Deployment.Port,
							},
						},
					}},
				},
			},
		}
		// Set owner reference 删除CRD，Deployment也会被删除
		if err := controllerutil.SetControllerReference(&app, deployment, r.Scheme); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "unable to ensure deployment is correct")
		app.Status.AvailableReplicas = 0
	} else {
		logger.Info("Deployment created or updated", "Name", deployment.Name)
		app.Status.AvailableReplicas = int(*deployment.Spec.Replicas)
	}
	return ctrl.Result{}, nil
}

// Create or update Service
func (r *ApplicationReconciler) createOrUpdateService(ctx context.Context, app applicationv1.Application, labels map[string]string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	// Create or update Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		service.Spec = corev1.ServiceSpec{
			Selector: labels,
			Ports:    app.Spec.Service.Ports,
		}

		// Set owner reference 删除CRD，Service也会被删除
		if err := controllerutil.SetControllerReference(&app, service, r.Scheme); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "unable to create or update Service")
	} else {
		logger.Info("Service created or updated", "Name", service.Name)
	}
	return ctrl.Result{}, err
}

// Create or update Ingress
func (r *ApplicationReconciler) createOrUpdateIngress(ctx context.Context, app applicationv1.Application, labels map[string]string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
			Labels:    labels,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ingress, func() error {
		ingress.Spec = networkingv1.IngressSpec{
			IngressClassName: app.Spec.Ingress.IngressClassName,
			Rules:            app.Spec.Ingress.Rules,
		}

		// Set owner reference 删除CRD，Ingress也会被删除
		if err := controllerutil.SetControllerReference(&app, ingress, r.Scheme); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "unable to create or update Ingress")
	} else {
		logger.Info("Ingress created or updated", "Name", ingress.Name)
	}
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&applicationv1.Application{}).
		Complete(r)
}

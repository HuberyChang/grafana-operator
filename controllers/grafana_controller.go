/*
Copyright 2023.

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
	"time"

	"k8s.io/apimachinery/pkg/util/intstr"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitorv1 "github.com/HuberyChang/grafana-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// GrafanaReconciler reconciles a Grafana object
type GrafanaReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=monitor.example.com,resources=grafanas,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitor.example.com,resources=grafanas/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=monitor.example.com,resources=grafanas/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Grafana object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *GrafanaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// TODO(user): your logic here
	instance := &monitorv1.Grafana{}

	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Grafana resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get Grafana")
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	componentName := instance.Spec.Component
	appName := instance.Name + "-" + componentName

	// 检查 grafana deployment是否存在
	dExist, err := r.deploymentIfNotExist(ctx, appName, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 如果 deployment 不存在，则创建
	if !dExist {
		if err := r.updateInstanceStatus(ctx, instance, "Creating grafana deployment"); err != nil {
			return ctrl.Result{}, err
		}

		grafanaDeploy := r.createDeployment(appName, instance.Namespace, 1)
		if err := controllerutil.SetControllerReference(instance, grafanaDeploy, r.Scheme); err != nil {
			_ = r.updateInstanceStatus(ctx, instance, "Failed to bind grafana deployment")
			logger.Error(err, "Error while binding grafana deployment")
			return ctrl.Result{}, err
		}

		if err := r.Client.Create(ctx, grafanaDeploy); err != nil {
			_ = r.updateInstanceStatus(ctx, instance, "Failed to create grafana deployment")
			logger.Error(err, "Error while creating grafana deployment")
			return ctrl.Result{}, err

		}

		if err := r.updateInstanceStatus(ctx, instance, "Created grafana deployment"); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil

	}

	// 检查 grafana service是否存在
	svcExist, err := r.serviceIfNotExist(ctx, appName, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 如果 service 不存在，则创建
	if !svcExist {
		if err := r.updateInstanceStatus(ctx, instance, "Creating grafana service"); err != nil {
			return ctrl.Result{}, err
		}

		grafanaSvc := r.createService(appName, instance.Namespace)
		if err := controllerutil.SetControllerReference(instance, grafanaSvc, r.Scheme); err != nil {
			_ = r.updateInstanceStatus(ctx, instance, "Failed to bind grafana service")
			logger.Error(err, "Error while binding grafana service")
			return ctrl.Result{}, err
		}

		if err := r.Client.Create(ctx, grafanaSvc); err != nil {
			_ = r.updateInstanceStatus(ctx, instance, "Failed to creating grafana service")
			logger.Error(err, "Error while creating grafana service")
			return ctrl.Result{}, err
		}

		if err := r.updateInstanceStatus(ctx, instance, "Created grafana service"); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil

	}

	if err := r.updateInstanceStatus(ctx, instance, "Successful"); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *GrafanaReconciler) deploymentIfNotExist(ctx context.Context, appName, namespace string) (bool, error) {
	logger := log.FromContext(ctx)
	_, err := r.getDeployment(ctx, appName, namespace)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		logger.Error(err, "Error while getting existed monitor deploy")
		return false, err
	}
	return true, nil
}

func (r *GrafanaReconciler) getDeployment(ctx context.Context, appName, namespace string) (*appsv1.Deployment, error) {
	logger := log.FromContext(ctx)

	deployment := &appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{
		Name:      appName,
		Namespace: namespace,
	}, deployment)
	if err != nil {
		logger.Error(err, "Error while checking existed monitor deploy")
		return nil, err
	}

	return deployment, nil
}

func (r *GrafanaReconciler) createDeployment(appName, namespace string, replicas int32) *appsv1.Deployment {
	appLabels := map[string]string{"app": appName}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: appLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: appLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "grafana",
							Image: "grafana/grafana",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 3000,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *GrafanaReconciler) serviceIfNotExist(ctx context.Context, appName, namespace string) (bool, error) {
	logger := log.FromContext(ctx)

	if _, err := r.getService(ctx, appName, namespace); err != nil {
		if errors.IsNotFound(err) {
			return false, err
		}
		logger.Error(err, "Error while checking existed grafana service")
		return false, err

	}
	return true, nil
}

func (r *GrafanaReconciler) getService(ctx context.Context, appName, namespace string) (*corev1.Service, error) {
	logger := log.FromContext(ctx)

	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      appName + "-SVC",
		Namespace: namespace,
	}, svc); err != nil {
		logger.Error(err, "Error while getting grafana service")
		return nil, err

	}
	return svc, nil
}

func (r *GrafanaReconciler) createService(appName, namespace string) *corev1.Service {
	appLabels := map[string]string{"app": appName}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName + "-service",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: appLabels,
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "web",
					Port:       3000,
					TargetPort: intstr.FromInt(3000),
				},
			},
		},
	}
}

func (r *GrafanaReconciler) updateInstanceStatus(ctx context.Context, instance *monitorv1.Grafana, result string) error {
	logger := log.FromContext(ctx)

	instance.Status.Result = result
	if err := r.Client.Status().Update(ctx, instance); err != nil {
		logger.Error(err, "Error while updating instance status")
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GrafanaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitorv1.Grafana{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

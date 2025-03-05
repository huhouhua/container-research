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
	"fmt"
	"github.com/robfig/cron/v3"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1 "github/huhouhua/container-research/cronhpa-operator/api/v1"
)

// CronHPAReconciler reconciles a CronHPA object
type CronHPAReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=autoscaling.aiops.com,resources=cronhpas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling.aiops.com,resources=cronhpas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaling.aiops.com,resources=cronhpas/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CronHPA object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *CronHPAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Reconciling CronHPA")
	var cronhpa autoscalingv1.CronHPA
	if err := r.Get(ctx, req.NamespacedName, &cronhpa); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("CronHPA resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	now := time.Now()
	var earliestNextRunTime *time.Time

	// 遍历 jobs，检查调度时间并更新目标工作负载副本数
	for _, job := range cronhpa.Spec.Jobs {
		lastRunTime := cronhpa.Status.LastRunTimes[job.Name]
		// 计算上次运行时间之后的下一个调度时间
		nextScheduledTime, err := r.getNextScheduledTime(job.Schedule, lastRunTime.Time)
		if err != nil {
			logger.Error(err, "Failed to calculate next scheduled time")
			return reconcile.Result{}, err
		}

		logger.Info("Job info", "name", job.Name, "lastRunTime", lastRunTime, "nextScheduledTime", nextScheduledTime, "now", now)

		// 检查当前时间是否已经到达或超过了计划的运行时间
		if now.After(nextScheduledTime) || now.Equal(nextScheduledTime) {
			// 更新副本数
			logger.Info("Updating deployment replicas", "name", cronhpa.Spec.ScaleTargetRef.Name, "targetSize", job.TargetSize)

			if err := r.updateDeploymentReplicas(ctx, &cronhpa, cronhpa.Spec.ScaleTargetRef, job); err != nil {
				return reconcile.Result{}, err
			}

			// 更新状态
			cronhpa.Status.CurrentReplicas = job.TargetSize
			cronhpa.Status.LastScaleTime = &metav1.Time{Time: now}

			// 更新作业的最后运行时间
			if cronhpa.Status.LastRunTimes == nil {
				cronhpa.Status.LastRunTimes = make(map[string]metav1.Time)
			}
			cronhpa.Status.LastRunTimes[job.Name] = metav1.Time{Time: now}

			// 计算下一次运行时间（从现在开始）
			nextRunTime, _ := r.getNextScheduledTime(job.Schedule, now)
			if earliestNextRunTime == nil || nextRunTime.Before(*earliestNextRunTime) {
				earliestNextRunTime = &nextRunTime
			}
		} else {
			// 如果当前时间未到达计划时间，将这个时间作为下一次运行时间
			if earliestNextRunTime == nil || nextScheduledTime.Before(*earliestNextRunTime) {
				earliestNextRunTime = &nextScheduledTime
			}
		}
	}

	// 更新 CronHPA 实例状态
	if err := r.Status().Update(ctx, &cronhpa); err != nil {
		return reconcile.Result{}, err
	}

	// 如果有下一次运行时间，设置重新入队
	if earliestNextRunTime != nil {
		requeueAfter := earliestNextRunTime.Sub(now)
		if requeueAfter < 0 {
			requeueAfter = time.Second // 如果计算出的时间已经过去，则在1秒后重新入队
		}
		logger.Info("Requeue after", "time", requeueAfter)
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

// updateDeploymentReplicas 更新目标工作负载的副本数
func (r *CronHPAReconciler) updateDeploymentReplicas(ctx context.Context, cronhpa *autoscalingv1.CronHPA, scaleTargetRef autoscalingv1.ScaleTargetReference, job autoscalingv1.JobSpec) error {
	logger := log.FromContext(ctx)

	// 创建 deployment 对象
	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{
		Name:      scaleTargetRef.Name,
		Namespace: cronhpa.Namespace,
	}
	jobSize := int32(job.TargetSize)

	// 获取 deployment
	if err := r.Get(ctx, deploymentKey, deployment); err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "Deployment not found", "deployment", deploymentKey)
		}
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// 检查当前副本数是否已经是目标副本数
	if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == jobSize {
		logger.Info("Deployment already at desired replica count", "deployment", deploymentKey, "replicas", job.TargetSize)
		return nil
	}

	// 更新副本数
	deployment.Spec.Replicas = &jobSize

	// 应用更新
	if err := r.Update(ctx, deployment); err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	logger.Info("Successfully updated deployment replicas", "deployment", deploymentKey, "replicas", job.TargetSize)

	return nil
}

// 获取下一个调度时间
func (r *CronHPAReconciler) getNextScheduledTime(schedule string, after time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	cronSchedule, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}, err
	}

	return cronSchedule.Next(after), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CronHPAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1.CronHPA{}).
		Complete(r)
}

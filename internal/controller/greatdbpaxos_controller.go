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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	greatdbv1 "greatdb.com/greatdb-operator/api/v1"
	"greatdb.com/greatdb-operator/internal/config"
	resources "greatdb.com/greatdb-operator/internal/controller/resourcemanager"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager/greatdbpaxos/core"
	dblog "greatdb.com/greatdb-operator/internal/utils/log"
)

// GreatDBPaxosReconciler reconciles a GreatDBPaxos object
type GreatDBPaxosReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups=core,resources=service,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=greatdb.greatdb.com,resources=greatdbpaxos,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=greatdb.greatdb.com,resources=greatdbpaxos/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=greatdb.greatdb.com,resources=greatdbpaxos/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GreatDBPaxos object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
// internal/controller/greatdbpaxos_controller.go
func (r *GreatDBPaxosReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	dblog.Log.Info("Start reconciling GreatDBCluster")

	// 1. 获取集群对象
	cluster := &greatdbv1.GreatDBPaxos{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			dblog.Log.Info("Cluster not found, may have been deleted")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.startForegroundDeletion(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// 2. 维护模式检查
	if cluster.Spec.MaintenanceMode {
		dblog.Log.Info("Cluster is in maintenance mode, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// 3. 设置默认值
	newCluster := cluster.DeepCopy()
	if setDefault(newCluster) {
		err := r.updateGreatDBCluster(ctx, newCluster)
		if err != nil {
			dblog.Log.Errorf("update cluster err %s", err.Error())
			return ctrl.Result{Requeue: false}, nil
		}

		dblog.Log.Info("Updated cluster defaults")
	}

	// 4. 同步资源（Secret/Service/ConfigMap 等）
	if err := r.syncClusterResources(ctx, newCluster); err != nil {
		dblog.Log.Errorf("sync cluster err %s", err.Error())
		return ctrl.Result{Requeue: false}, err
	}

	if err := r.updateGreatDBCluster(ctx, newCluster); err != nil {
		dblog.Log.Errorf("update cluster err %s", err.Error())

		return ctrl.Result{Requeue: false}, nil
	}

	dblog.Log.Info("Updated cluster spec")

	// 5. 更新状态
	if !reflect.DeepEqual(cluster.Status, newCluster.Status) {
		if err := r.Status().Update(ctx, newCluster); err != nil {
			return ctrl.Result{}, err
		}
		dblog.Log.Info("Updated cluster status")
	}

	dblog.Log.Infof("Successfully synchronized cluster %s/%s", cluster.Namespace, cluster.Name)

	return ctrl.Result{}, nil
}

func (r *GreatDBPaxosReconciler) updateGreatDBCluster(ctx context.Context, cluster *greatdbv1.GreatDBPaxos) error {

	// 创建 cluster 的深拷贝用于比较
	originalCluster := cluster.DeepCopy()

	// 使用 controller-runtime 的 Update 方法重试更新
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// 尝试更新 cluster
		if err := r.Client.Update(ctx, cluster); err != nil {
			if !apierrors.IsConflict(err) {
				return err
			}

			// 处理冲突时获取最新版本
			latestCluster := &greatdbv1.GreatDBPaxos{}
			if err = r.Client.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: cluster.Name}, latestCluster); err != nil {
				dblog.Log.Errorf("failed to get latest greatdb-cluster %s/%s: %v", cluster.Namespace, cluster.Name, err)
				return err
			}

			// 合并元数据，保持原有逻辑
			cluster = latestCluster.DeepCopy()
			cluster.Labels = resources.MergeLabels(cluster.Labels, originalCluster.Labels)
			cluster.Annotations = resources.MergeAnnotation(cluster.Annotations, originalCluster.Annotations)
			cluster.Finalizers = originalCluster.Finalizers

			return fmt.Errorf("conflict occurred, retrying")
		}
		return nil
	})

	if err != nil {
		dblog.Log.Reason(err).Errorf("failed to update cluster %s/%s",
			cluster.Namespace, cluster.Name)
		return err
	}

	return nil
}

// updateCluster Synchronize the cluster state to the desired state
func (r *GreatDBPaxosReconciler) syncClusterResources(ctx context.Context, cluster *greatdbv1.GreatDBPaxos) (err error) {

	managers := core.NewGreatDBPaxosResourceManagers(r.Client, r.Recorder, r.Scheme)
	// synchronize cluster secret
	if err = managers.SecretManager.Sync(ctx, cluster); err != nil {
		dblog.Log.Errorf("Failed to synchronize secret, message: %s ", err.Error())
		return err
	}

	// Synchronize service
	if err = managers.ServiceManager.Sync(ctx, cluster); err != nil {
		dblog.Log.Errorf("Failed to synchronize service, message: %s ", err.Error())
		return err
	}

	// Synchronize configmap
	if err = managers.ConfigMapManager.Sync(ctx, cluster); err != nil {
		dblog.Log.Errorf("Failed to synchronize configmap, message: %s ", err.Error())
		return err
	}

	// Synchronize GreatDB
	if err = managers.GreatDBPaxosManager.Sync(ctx, cluster); err != nil {
		dblog.Log.Errorf("Failed to synchronize GreatDB , message: %s ", err.Error())
		return err
	}

	if err = managers.DashboardManager.Sync(ctx, cluster); err != nil {
		return err
	}

	if err = r.startForegroundDeletion(ctx, cluster); err != nil {
		return err
	}

	return nil
}

func (r *GreatDBPaxosReconciler) startForegroundDeletion(ctx context.Context, cluster *greatdbv1.GreatDBPaxos) error {

	if cluster.DeletionTimestamp.IsZero() {
		return nil
	}

	if len(cluster.Finalizers) == 0 {
		return nil
	}
	patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
	err := r.Client.Patch(ctx, cluster, client.RawPatch(types.JSONPatchType, []byte(patch)))
	if err != nil {
		return err
	}
	return nil
}

func setDefault(cluster *greatdbv1.GreatDBPaxos) bool {
	if !cluster.DeletionTimestamp.IsZero() {
		return false
	}
	update := false
	if cluster.Spec.ImagePullPolicy == "" {
		update = true
		cluster.Spec.ImagePullPolicy = corev1.PullIfNotPresent
	}

	if cluster.Spec.ClusterDomain == "" {
		cluster.Spec.ClusterDomain = config.DefaultClusterDomain
	}

	// set GreatDb component
	if SetGreatDB(cluster) {
		update = true
	}
	if SetGreatDBAgent(cluster) {
		update = true
	}

	if SetDashboard(cluster) {
		update = true
	}

	if SetLog(cluster) {
		update = true
	}

	// set metadata
	if SetMeta(cluster) {
		update = true
	}

	if SetSecurityContext(cluster) {
		update = true
	}
	return update
}

// SetMeta Configuration Label、 annotations and ownerReferences
func SetMeta(cluster *greatdbv1.GreatDBPaxos) bool {
	update := false

	// set Label"k8s.io/apimachinery/pkg/api/resource"
	if cluster.Labels == nil {
		cluster.Labels = make(map[string]string)
	}

	if _, ok := cluster.Labels[resources.AppKubeNameLabelKey]; !ok {
		cluster.Labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
		update = true
	}

	if _, ok := cluster.Labels[resources.AppkubeManagedByLabelKey]; !ok {
		update = true
		cluster.Labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	}

	if cluster.Finalizers == nil {
		cluster.Finalizers = make([]string, 0)
	}
	existFinalizers := false
	for _, v := range cluster.Finalizers {
		if v == resources.FinalizersGreatDBCluster {
			existFinalizers = true
		}
	}
	if !existFinalizers {
		update = true
		cluster.Finalizers = append(cluster.Finalizers, resources.FinalizersGreatDBCluster)
	}

	return update

}

func SetGreatDB(cluster *greatdbv1.GreatDBPaxos) bool {
	update := false

	if cluster.Spec.Service.Type == "" {
		cluster.Spec.Service.Type = corev1.ServiceTypeClusterIP
	}

	// restart

	if cluster.Spec.Restart == nil {
		cluster.Spec.Restart = &greatdbv1.RestartGreatDB{}
	}

	if cluster.Spec.Restart.Mode != greatdbv1.ClusterRestart {
		cluster.Spec.Restart.Mode = greatdbv1.InsRestart
	}
	if cluster.Spec.Restart.Strategy != greatdbv1.AllRestart {
		cluster.Spec.Restart.Strategy = greatdbv1.RollingRestart
	}

	if cluster.Spec.UpgradeStrategy != greatdbv1.AllUpgrade {
		cluster.Spec.UpgradeStrategy = greatdbv1.RollingUpgrade
	}

	if cluster.Spec.Scaling == nil {
		cluster.Spec.Scaling = &greatdbv1.Scaling{
			ScaleIn: greatdbv1.ScaleIn{
				Strategy: greatdbv1.ScaleInStrategyIndex,
			},
		}
	}

	if cluster.Spec.Scaling.ScaleOut.Source == "" {
		cluster.Spec.Scaling.ScaleOut.Source = greatdbv1.ScaleOutSourceBackup
	}

	if cluster.Spec.Instances < 2 {
		update = true
		cluster.Spec.Instances = 2
	}

	if cluster.Spec.Port == 0 {
		cluster.Spec.Port = 3306
	}
	maxUnavailable := intstr.FromInt(0)
	if cluster.Spec.FailOver.MaxUnavailable == nil {
		cluster.Spec.FailOver.MaxUnavailable = &maxUnavailable
		update = true
	}

	if cluster.Spec.FailOver.Enable == nil {
		update = true
		cluster.Spec.FailOver.Enable = &update
	}

	if cluster.Spec.FailOver.AutoScaleIn == nil {
		update = true
		scaleIn := false
		cluster.Spec.FailOver.AutoScaleIn = &scaleIn
	}

	if cluster.Spec.Resources.Requests == nil {
		cluster.Spec.Resources.Requests = make(corev1.ResourceList)
	}

	if cluster.Spec.Resources.Limits == nil {
		cluster.Spec.Resources.Limits = make(corev1.ResourceList)
	}

	// Check the cpu memory and set the minimum value
	if cluster.Spec.Resources.Requests.Cpu().IsZero() {
		update = true
		cluster.Spec.Resources.Requests[corev1.ResourceCPU] = *resource.NewQuantity(2, resource.BinarySI)
	}

	if cluster.Spec.Resources.Limits.Cpu().Cmp(*cluster.Spec.Resources.Requests.Cpu()) == -1 {
		update = true
		cluster.Spec.Resources.Limits[corev1.ResourceCPU] = cluster.Spec.Resources.Requests[corev1.ResourceCPU]
	}

	if cluster.Spec.Resources.Requests.Memory().IsZero() {
		update = true
		cluster.Spec.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("2Gi")
	}

	if cluster.Spec.Resources.Limits.Memory().Cmp(*cluster.Spec.Resources.Requests.Memory()) == -1 {
		update = true
		cluster.Spec.Resources.Limits[corev1.ResourceMemory] = cluster.Spec.Resources.Requests[corev1.ResourceMemory]
	}

	pvc := cluster.Spec.VolumeClaimTemplates
	size := pvc.Resources.Requests.Storage().String()

	pvc, change := SetVolumeClaimTemplates(size, pvc.StorageClassName, pvc.AccessModes)
	if change {
		update = true
		cluster.Spec.VolumeClaimTemplates = pvc
	}

	return update

}

func SetVolumeClaimTemplates(size string, storageClassName *string, AccessMode []corev1.PersistentVolumeAccessMode) (corev1.PersistentVolumeClaimSpec, bool) {

	update := false
	if size == "0" || size == "" {
		size = "10Gi"
		update = true
	}

	if len(AccessMode) == 0 {
		AccessMode = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		update = true
	}

	volume := corev1.PersistentVolumeClaimSpec{
		AccessModes:      AccessMode,
		StorageClassName: storageClassName,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(size),
			}},
	}
	return volume, update

}

func SetSecurityContext(cluster *greatdbv1.GreatDBPaxos) bool {
	// var runAsGroup  int64 = 1000
	var runAsUser int64 = 1000
	var fsGroup int64 = 1000
	update := false

	// if config.ServerVersion >= "1.14.0" {
	// 	if cluster.Spec.PodSecurityContext.FSGroupChangePolicy == nil {
	// 		policy := corev1.FSGroupChangeOnRootMismatch
	// 		cluster.Spec.PodSecurityContext.FSGroupChangePolicy = &policy
	// 		update = true
	// 	}
	// }

	// fsgroup
	if cluster.Spec.PodSecurityContext.RunAsGroup == nil && cluster.Spec.PodSecurityContext.FSGroup == nil {
		cluster.Spec.PodSecurityContext.FSGroup = &fsGroup
		return true
	}

	if cluster.Spec.PodSecurityContext.RunAsGroup != nil && *cluster.Spec.PodSecurityContext.RunAsGroup != *cluster.Spec.PodSecurityContext.FSGroup {
		cluster.Spec.PodSecurityContext.FSGroup = cluster.Spec.PodSecurityContext.RunAsGroup
		update = true
	}

	if cluster.Spec.PodSecurityContext.RunAsGroup == nil && *cluster.Spec.PodSecurityContext.FSGroup != fsGroup {
		cluster.Spec.PodSecurityContext.FSGroup = &fsGroup
		update = true
	}

	// runAsUser
	if cluster.Spec.PodSecurityContext.RunAsGroup == nil {
		cluster.Spec.PodSecurityContext.RunAsUser = &runAsUser
		update = true
	}

	return update

}

func SetGreatDBAgent(cluster *greatdbv1.GreatDBPaxos) bool {
	update := false

	if cluster.Spec.Backup.Enable == nil {
		enable := true
		cluster.Spec.Backup.Enable = &enable
		update = true
	}

	if !*cluster.Spec.Backup.Enable {
		return update
	}

	if cluster.Spec.Backup.Resources.Requests == nil {
		cluster.Spec.Backup.Resources.Requests = make(corev1.ResourceList)
	}

	if cluster.Spec.Backup.Resources.Limits == nil {
		cluster.Spec.Backup.Resources.Limits = make(corev1.ResourceList)
	}

	// Check the cpu memory and set the minimum value
	if cluster.Spec.Backup.Resources.Requests.Cpu().IsZero() {
		update = true
		cluster.Spec.Backup.Resources.Requests[corev1.ResourceCPU] = *resource.NewQuantity(1, resource.BinarySI)
	}

	if cluster.Spec.Backup.Resources.Limits.Cpu().Cmp(*cluster.Spec.Backup.Resources.Requests.Cpu()) == -1 {
		update = true
		cluster.Spec.Backup.Resources.Limits[corev1.ResourceCPU] = cluster.Spec.Backup.Resources.Requests[corev1.ResourceCPU]
	}

	if cluster.Spec.Backup.Resources.Requests.Memory().IsZero() {
		update = true
		cluster.Spec.Backup.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("1Gi")
	}

	if cluster.Spec.Backup.Resources.Limits.Memory().Cmp(*cluster.Spec.Backup.Resources.Requests.Memory()) == -1 {
		update = true
		cluster.Spec.Backup.Resources.Limits[corev1.ResourceMemory] = cluster.Spec.Backup.Resources.Requests[corev1.ResourceMemory]
	}

	return update
}

func SetDashboard(cluster *greatdbv1.GreatDBPaxos) bool {
	update := false

	if !cluster.Spec.Dashboard.Enable {
		return false
	}

	if cluster.Spec.Dashboard.Resources.Requests == nil {
		cluster.Spec.Dashboard.Resources.Requests = make(corev1.ResourceList)
	}

	if cluster.Spec.Dashboard.Resources.Limits == nil {
		cluster.Spec.Dashboard.Resources.Limits = make(corev1.ResourceList)
	}

	// Check the cpu memory and set the minimum value
	if cluster.Spec.Dashboard.Resources.Requests.Cpu().IsZero() {
		update = true
		cluster.Spec.Dashboard.Resources.Requests[corev1.ResourceCPU] = *resource.NewQuantity(2, resource.BinarySI)
	}

	if cluster.Spec.Dashboard.Resources.Limits.Cpu().Cmp(*cluster.Spec.Dashboard.Resources.Requests.Cpu()) == -1 {
		update = true
		cluster.Spec.Dashboard.Resources.Limits[corev1.ResourceCPU] = cluster.Spec.Dashboard.Resources.Requests[corev1.ResourceCPU]
	}

	if cluster.Spec.Dashboard.Resources.Requests.Memory().IsZero() {
		update = true
		cluster.Spec.Dashboard.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("2Gi")
	}

	if cluster.Spec.Dashboard.Resources.Limits.Memory().Cmp(*cluster.Spec.Dashboard.Resources.Requests.Memory()) == -1 {
		update = true
		cluster.Spec.Dashboard.Resources.Limits[corev1.ResourceMemory] = cluster.Spec.Dashboard.Resources.Requests[corev1.ResourceMemory]
	}
	pvc := cluster.Spec.Dashboard.PersistentVolumeClaimSpec
	size := pvc.Resources.Requests.Storage().String()

	pvc, change := SetVolumeClaimTemplates(size, pvc.StorageClassName, pvc.AccessModes)
	if change {
		update = true
		cluster.Spec.Dashboard.PersistentVolumeClaimSpec = pvc
	}

	return update
}

func SetLog(cluster *greatdbv1.GreatDBPaxos) bool {
	update := false

	if cluster.Spec.LogCollection.Image == "" {
		return false
	}

	if cluster.Spec.LogCollection.Resources.Requests == nil {
		cluster.Spec.LogCollection.Resources.Requests = make(corev1.ResourceList)
	}

	if cluster.Spec.LogCollection.Resources.Limits == nil {
		cluster.Spec.LogCollection.Resources.Limits = make(corev1.ResourceList)
	}

	// Check the cpu memory and set the minimum value
	if cluster.Spec.LogCollection.Resources.Requests.Cpu().IsZero() {
		update = true
		cluster.Spec.LogCollection.Resources.Requests[corev1.ResourceCPU] = resource.MustParse("300m")
	}

	if cluster.Spec.LogCollection.Resources.Limits.Cpu().Cmp(*cluster.Spec.LogCollection.Resources.Requests.Cpu()) == -1 {
		update = true
		cluster.Spec.LogCollection.Resources.Limits[corev1.ResourceCPU] = cluster.Spec.LogCollection.Resources.Requests[corev1.ResourceCPU]
	}

	if cluster.Spec.LogCollection.Resources.Requests.Memory().IsZero() {
		update = true
		cluster.Spec.LogCollection.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("300Mi")
	}

	if cluster.Spec.LogCollection.Resources.Limits.Memory().Cmp(*cluster.Spec.LogCollection.Resources.Requests.Memory()) == -1 {
		update = true
		cluster.Spec.LogCollection.Resources.Limits[corev1.ResourceMemory] = cluster.Spec.LogCollection.Resources.Requests[corev1.ResourceMemory]
	}

	return update
}

// SetupWithManager sets up the controller with the Manager.
func (r *GreatDBPaxosReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&greatdbv1.GreatDBPaxos{}).
		Named("greatdbpaxos").
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

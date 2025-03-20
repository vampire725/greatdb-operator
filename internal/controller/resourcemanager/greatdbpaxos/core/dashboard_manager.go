package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	greatdbv1 "greatdb.com/greatdb-operator/api/v1"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager"
	dblog "greatdb.com/greatdb-operator/internal/utils/log"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
)

type DashboardManager struct {
	Client   client.Client
	Recorder record.EventRecorder
}

func (dashboard *DashboardManager) Sync(ctx context.Context, cluster *greatdbv1.GreatDBPaxos) (err error) {

	if !cluster.Spec.Dashboard.Enable {
		return nil
	}

	if cluster.Status.Status != greatdbv1.ClusterStatusOnline {
		return nil
	}

	if err = dashboard.syncPvc(ctx, cluster); err != nil {
		return err
	}

	if err = dashboard.CreateOrUpdateDashboard(ctx, cluster); err != nil {
		return err
	}

	return nil

}

func (dashboard *DashboardManager) SyncClusterTopo(cluster *greatdbv1.GreatDBPaxos) error {

	if metav1.Now().Sub(cluster.Status.Dashboard.LastSyncTime.Time) < time.Minute*1 {
		return nil
	}

	serverAddr := cluster.Name + resourcemanager.ServiceWrite
	// Get the cluster account password from secret
	user, password := resourcemanager.GetClusterUser(cluster)

	syncCluster := make(map[string]interface{})
	syncCluster["address"] = serverAddr
	syncCluster["port"] = fmt.Sprintf("%d", cluster.Spec.Port)
	syncCluster["cluster_name"] = cluster.Name
	syncCluster["username"] = user
	syncCluster["password"] = password
	syncCluster["gdbc_type"] = "mgr"
	bytesData, err := json.Marshal(syncCluster)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	reader := bytes.NewReader(bytesData)

	dashboardName := cluster.Name + resourcemanager.ComponentDashboardSuffix
	syncUrlFmt := "http://%s.%s.svc.%s:8080/gdbc/api/v1/cluster/init_cluster/"
	dashboardSyncUrl := fmt.Sprintf(syncUrlFmt, dashboardName, cluster.Namespace, cluster.GetClusterDomain())

	// dashboardSyncUrl = "http://172.17.120.143:30500/gdbc/api/v1/cluster/init_cluster/"

	request, err := http.NewRequest("POST", dashboardSyncUrl, reader)
	if err != nil {
		dblog.Log.Error(err.Error())
		return err
	}
	request.Header.Set("Content-Type", "application/json;charset=UTF-8")
	httpClient := http.Client{}
	resp, err := httpClient.Do(request)
	if err != nil {
		dblog.Log.Error(err.Error())
		return err
	}
	syncRes := fmt.Sprintf("Sync cluster response StatusCode %d:", resp.StatusCode)
	if resp.StatusCode == 200 {
		cluster.Status.Dashboard.Ready = true
		cluster.Status.Dashboard.LastSyncTime = metav1.Now()
	} else {
		cluster.Status.Dashboard.Ready = false
	}
	dblog.Log.Info(syncRes)

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	return nil
}

func (dashboard *DashboardManager) CreateOrUpdateDashboard(ctx context.Context, cluster *greatdbv1.GreatDBPaxos) error {

	dashBoardName := cluster.Name + resourcemanager.ComponentDashboardSuffix
	ns := cluster.GetNamespace()
	var pod = &corev1.Pod{}
	err := dashboard.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: dashBoardName}, pod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// create dashboard pods
			if err = dashboard.createDashboard(ctx, cluster); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Errorf("failed to sync greatDB statefulset of cluster %s/%s", ns, cluster.Name)
		return err
	}
	newPod := pod.DeepCopy()
	if err = dashboard.updateDashboard(ctx, cluster, newPod); err != nil {
		return err
	}

	for _, con := range pod.Status.Conditions {
		if con.Status == corev1.ConditionTrue && con.Type == corev1.ContainersReady {
			if err = dashboard.SyncClusterTopo(cluster); err == nil {
				return nil
			}
			break
		}
	}

	dblog.Log.Infof("Successfully update the greatdb-dashboard pod %s/%s", ns, dashBoardName)

	return nil
}

func (dashboard *DashboardManager) createDashboard(ctx context.Context, cluster *greatdbv1.GreatDBPaxos) error {
	// The cluster starts to clean, and no more resources need to be created
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}
	pod := dashboard.newDashboardPod(cluster)

	if pod == nil {
		return nil
	}

	err := dashboard.Client.Create(ctx, pod)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			labels, _ := json.Marshal(pod.ObjectMeta.Labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labels)
			err = dashboard.Client.Patch(ctx, pod, client.RawPatch(types.StrategicMergePatchType, []byte(data)))
			if err != nil {
				dblog.Log.Errorf("failed to update the labels of statefulset, message: %s", err.Error())
				return err
			}
			return nil
		}

		dblog.Log.Errorf("failed to create  the dashboard  pods %s/%s  , message: %s", pod.Namespace, pod.Name, err.Error())
		return err
	}

	dblog.Log.Infof("Successfully created the dashboard  pods %s/%s", pod.Namespace, pod.Name)

	return nil

}

// GetLabels  Return to the default label settings
func (dashboard *DashboardManager) GetLabels(name string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resourcemanager.AppKubeNameLabelKey] = resourcemanager.AppKubeNameLabelValue
	labels[resourcemanager.AppkubeManagedByLabelKey] = resourcemanager.AppkubeManagedByLabelValue
	labels[resourcemanager.AppKubeInstanceLabelKey] = name
	labels[resourcemanager.AppKubeComponentLabelKey] = resourcemanager.AppKubeComponentDashboard

	return

}

func (dashboard *DashboardManager) newDashboardPod(cluster *greatdbv1.GreatDBPaxos) (pod *corev1.Pod) {
	name := cluster.Name + resourcemanager.ComponentDashboardSuffix
	serviceName := cluster.Name + resourcemanager.ComponentDashboardSuffix

	initContainers := dashboard.newInitContainers(serviceName, cluster)
	containers := dashboard.newContainers(serviceName, cluster)
	volume := dashboard.newVolumes(cluster, name)
	owner := resourcemanager.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
	// update Affinity
	affinity := cluster.Spec.Affinity
	if cluster.Spec.Affinity != nil {
		affinity = cluster.Spec.Affinity
	}
	if affinity == nil {
		affinity = getDefaultAffinity(cluster.Name)
	}

	pod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       cluster.Namespace,
			Finalizers:      []string{resourcemanager.FinalizersGreatDBCluster},
			OwnerReferences: []metav1.OwnerReference{owner},
			Labels:          dashboard.GetLabels(cluster.Name),
		},
		Spec: corev1.PodSpec{
			InitContainers:    initContainers,
			Containers:        containers,
			Volumes:           volume,
			PriorityClassName: cluster.Spec.PriorityClassName,
			SecurityContext:   &cluster.Spec.PodSecurityContext,
			ImagePullSecrets:  cluster.Spec.ImagePullSecrets,
			Affinity:          affinity,
		},
	}

	return

}

func (dashboard *DashboardManager) getVolumeClaimTemplates(cluster *greatdbv1.GreatDBPaxos) (pvcs []corev1.PersistentVolumeClaim) {

	if !cluster.Spec.Dashboard.PersistentVolumeClaimSpec.Resources.Requests.Storage().IsZero() {
		pvcs = append(pvcs, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: resourcemanager.GreatdbPvcDataName,
			},
			Spec: cluster.Spec.Dashboard.PersistentVolumeClaimSpec,
		})
	}
	return

}

func (dashboard *DashboardManager) newInitContainers(serviceName string, cluster *greatdbv1.GreatDBPaxos) (containers []corev1.Container) {

	init := dashboard.newInitDashboardContainers(serviceName, cluster)

	containers = append(containers, init)

	return
}

func (dashboard *DashboardManager) newContainers(serviceName string, cluster *greatdbv1.GreatDBPaxos) (containers []corev1.Container) {

	db := dashboard.newDashboardContainers(serviceName, cluster)
	containers = append(containers, db)
	return
}

func (dashboard *DashboardManager) newDashboardContainers(serviceName string, cluster *greatdbv1.GreatDBPaxos) (container corev1.Container) {
	clusterDomain := cluster.GetClusterDomain()

	env := dashboard.newDashboardEnv(serviceName, clusterDomain, cluster)
	envForm := dashboard.newSecretEnvForm(cluster.Spec.SecretName)
	imagePullPolicy := corev1.PullIfNotPresent
	if cluster.Spec.ImagePullPolicy != "" {
		imagePullPolicy = cluster.Spec.ImagePullPolicy
	}

	container = corev1.Container{
		Name:            DashboardContainerName,
		Env:             env,
		EnvFrom:         envForm,
		Image:           cluster.Spec.Dashboard.Image,
		Resources:       cluster.Spec.Dashboard.Resources,
		ImagePullPolicy: imagePullPolicy,
		VolumeMounts: []corev1.VolumeMount{
			{ // data
				Name:      resourcemanager.GreatdbPvcDataName,
				MountPath: dashboardDataMountPath,
			},
		},
		ReadinessProbe: &corev1.Probe{
			PeriodSeconds:       10,
			InitialDelaySeconds: 20,
			FailureThreshold:    3,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(8080)},
			},
		},
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: 15,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(8080)},
			},
		},
	}

	return
}

func (dashboard *DashboardManager) newInitDashboardContainers(serviceName string, cluster *greatdbv1.GreatDBPaxos) (container corev1.Container) {
	clusterDomain := cluster.GetClusterDomain()
	env := dashboard.newDashboardEnv(serviceName, clusterDomain, cluster)
	envForm := dashboard.newSecretEnvForm(cluster.Spec.SecretName)
	imagePullPolicy := corev1.PullIfNotPresent
	if cluster.Spec.ImagePullPolicy != "" {
		imagePullPolicy = cluster.Spec.ImagePullPolicy
	}
	container = corev1.Container{
		Name:            DashboardContainerName + "-init",
		Env:             env,
		EnvFrom:         envForm,
		Command:         []string{"sh", "scripts/prestart.sh"},
		WorkingDir:      "/app",
		Image:           cluster.Spec.Dashboard.Image,
		Resources:       cluster.Spec.Dashboard.Resources,
		ImagePullPolicy: imagePullPolicy,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      resourcemanager.GreatdbPvcDataName,
				MountPath: dashboardDataMountPath,
			},
		},
	}

	return
}

func (dashboard *DashboardManager) newDashboardEnv(serviceName, clusterDomain string, cluster *greatdbv1.GreatDBPaxos) (env []corev1.EnvVar) {
	monitorRemoteWrite := cluster.Spec.Dashboard.Config["monitorRemoteWrite"]
	permissionCheckUrl := cluster.Spec.Dashboard.Config["permissionCheckUrl"]
	lokiRemoteStorage := cluster.Spec.Dashboard.Config["lokiRemoteStorage"]
	if !strings.HasPrefix(permissionCheckUrl, "http") {
		permissionCheckUrl = "http://localhost:8099/gdbc/api/v1/cluster/init_cluster/verifyPermission"
	}

	env = []corev1.EnvVar{
		{
			Name: "PODNAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.name",
				},
			},
		},
		{
			Name: "NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.namespace",
				},
			},
		},
		{
			Name:  "SERVICE_NAME",
			Value: serviceName,
		},
		{
			Name:  "FQDN",
			Value: "$(PODNAME).$(SERVICE_NAME).$(NAMESPACE).svc." + clusterDomain,
		},
		{
			Name:  "ADM_ADDRESS",
			Value: "127.0.0.1",
		},
		{
			Name:  "DAS_ADDRESS",
			Value: "127.0.0.1",
		},
		{
			Name:  "ADM_WEB_PORT",
			Value: "8080",
		},
		{
			Name:  "MONITOR_DATA_MAXIMUM_TIME",
			Value: "16d",
		},
		{
			Name:  "MONITOR_DATA_MAXIMUM_SIZE",
			Value: "20GB",
		},
		{
			Name:  "MAX_WORKERS",
			Value: "2",
		},
		{
			Name:  "DAS_MAX_WORKERS",
			Value: "2",
		},
		{
			Name:  "JXJK_PERMISSION_CHECK_URL",
			Value: permissionCheckUrl,
		},
		{
			Name:  "INTERNAL_ADM_WEB_PORT",
			Value: "8099",
		},
		{
			Name:  "MONITOR_REMOTE_WRITE",
			Value: monitorRemoteWrite,
		},
		{
			Name:  "LOKI_REMOTE_STORAGE",
			Value: lokiRemoteStorage,
		},
	}
	return
}

func (dashboard *DashboardManager) newSecretEnvForm(secretName string) (env []corev1.EnvFromSource) {
	env = []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
			},
		},
	}
	return env
}

func (dashboard *DashboardManager) newVolumes(cluster *greatdbv1.GreatDBPaxos, pvcName string) (volumes []corev1.Volume) {
	if cluster.Spec.Dashboard.PersistentVolumeClaimSpec.Resources.Requests.Storage().IsZero() {
		volumes = []corev1.Volume{
			{
				Name: resourcemanager.GreatdbPvcDataName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		}
		return volumes
	}

	volumes = []corev1.Volume{
		{
			Name: resourcemanager.GreatdbPvcDataName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		},
	}
	return volumes
}

func (dashboard *DashboardManager) updateDashboard(ctx context.Context, cluster *greatdbv1.GreatDBPaxos, podIns *corev1.Pod) error {

	if !cluster.DeletionTimestamp.IsZero() {
		if len(podIns.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
		err := dashboard.Client.Patch(ctx, podIns, client.RawPatch(types.JSONPatchType, []byte(patch)))
		if err != nil {
			dblog.Log.Reason(err).Errorf("failed to delete finalizers of pod %s/%s", podIns.Namespace, podIns.Name)
		}
		return nil
	}

	needUpdate := false
	// update labels
	if up := dashboard.updateLabels(podIns, cluster); up {
		needUpdate = true
	}

	// update annotations
	if up := dashboard.updateAnnotations(podIns, cluster); up {
		needUpdate = true
	}

	// update Finalizers
	if podIns.ObjectMeta.Finalizers == nil {
		podIns.ObjectMeta.Finalizers = make([]string, 0, 1)
	}

	exist := false
	for _, value := range podIns.ObjectMeta.Finalizers {
		if value == resourcemanager.FinalizersGreatDBCluster {
			exist = true
			break
		}
	}
	if !exist {
		podIns.ObjectMeta.Finalizers = append(podIns.ObjectMeta.Finalizers, resourcemanager.FinalizersGreatDBCluster)
		needUpdate = true
	}

	// update OwnerReferences
	if podIns.ObjectMeta.OwnerReferences == nil {
		podIns.ObjectMeta.OwnerReferences = make([]metav1.OwnerReference, 0, 1)
	}
	exist = false
	for _, value := range podIns.ObjectMeta.OwnerReferences {
		if value.UID == cluster.UID {
			exist = true
			break
		}
	}
	if !exist {
		owner := resourcemanager.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
		podIns.ObjectMeta.OwnerReferences = []metav1.OwnerReference{owner}
		needUpdate = true
	}

	if needUpdate {

		err := dashboard.updatePod(ctx, podIns)
		if err != nil {
			return err
		}

	}

	return nil
}

func (dashboard *DashboardManager) updatePod(ctx context.Context, pod *corev1.Pod) error {

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := dashboard.Client.Update(ctx, pod)
		if err == nil {
			return nil
		}
		var upPod = &corev1.Pod{}
		err1 := dashboard.Client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}, upPod)
		if err1 != nil {
			dblog.Log.Reason(err).Errorf("failed to update pod %s/%s ", pod.Namespace, pod.Name)
		} else {
			if pod.ResourceVersion != upPod.ResourceVersion {
				pod.ResourceVersion = upPod.ResourceVersion
			}
		}

		return err
	})
	return err
}

func (dashboard *DashboardManager) updateLabels(pod *corev1.Pod, cluster *greatdbv1.GreatDBPaxos) bool {
	needUpdate := false
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	labels := resourcemanager.MergeLabels(cluster.Spec.Labels, dashboard.GetLabels(cluster.Name))
	for key, value := range labels {
		if v, ok := pod.Labels[key]; !ok || v != value {
			pod.Labels[key] = value
			needUpdate = true
		}
	}

	return needUpdate
}

func (dashboard *DashboardManager) updateAnnotations(pod *corev1.Pod, cluster *greatdbv1.GreatDBPaxos) bool {
	needUpdate := false
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	for key, value := range cluster.Spec.Annotations {
		if v, ok := pod.Annotations[key]; !ok || v != value {
			pod.Annotations[key] = value
			needUpdate = true
		}
	}

	return needUpdate
}

func (dashboard *DashboardManager) syncPvc(ctx context.Context, cluster *greatdbv1.GreatDBPaxos) error {
	pvcName := cluster.Name + resourcemanager.ComponentDashboardSuffix
	var pvc = &corev1.PersistentVolumeClaim{}
	err := dashboard.Client.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: pvcName}, pvc)
	if err != nil {

		if k8serrors.IsNotFound(err) {
			return dashboard.CreatePvc(ctx, cluster, pvcName)
		}
		dblog.Log.Reason(err).Errorf("failed to lister pvc %s/%s", cluster.Namespace, pvcName)

	}
	newPvc := pvc.DeepCopy()

	great := GreatDBPaxosManager{Client: dashboard.Client, Recorder: dashboard.Recorder}

	err = great.UpdatePvc(ctx, cluster, newPvc)
	if err != nil {
		return err
	}

	err = great.updatePv(ctx, newPvc, cluster.Spec.PvReclaimPolicy)
	if err != nil {
		return err
	}

	return nil

}

func (dashboard *DashboardManager) CreatePvc(ctx context.Context, cluster *greatdbv1.GreatDBPaxos, pvcName string) error {

	pvc := dashboard.NewPvc(cluster, pvcName)
	err := dashboard.Client.Create(ctx, pvc)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			//  need to add a label to the configmap to ensure that the operator can monitor it
			labels, _ := json.Marshal(pvc.Labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labels)
			err = dashboard.Client.Patch(ctx, pvc, client.RawPatch(types.StrategicMergePatchType, []byte(data)))
			if err != nil {
				dblog.Log.Errorf("failed to update the labels of pod, message: %s", err.Error())
				return err
			}
			return nil
		}
	}
	dblog.Log.Infof("successfully created PVC %s/%s", pvc.Namespace, pvc.Name)

	return nil

}

func (dashboard *DashboardManager) NewPvc(cluster *greatdbv1.GreatDBPaxos, pvcName string) (pvc *corev1.PersistentVolumeClaim) {

	owner := resourcemanager.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            pvcName,
			Namespace:       cluster.Namespace,
			Labels:          dashboard.GetLabels(cluster.Name),
			Finalizers:      []string{resourcemanager.FinalizersGreatDBCluster},
			OwnerReferences: []metav1.OwnerReference{owner},
			Annotations:     cluster.Spec.Annotations,
		},
		Spec: cluster.Spec.Dashboard.PersistentVolumeClaimSpec,
	}
	return

}

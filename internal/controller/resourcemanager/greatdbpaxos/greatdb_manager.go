package greatdbpaxos

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"greatdb.com/greatdb-operator/internal/config"

	v1alpha1 "greatdb.com/greatdb-operator/api/v1"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager"
	dblog "greatdb.com/greatdb-operator/internal/utils/log"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GreatDBManager struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

func (great *GreatDBManager) Sync(cluster *v1alpha1.GreatDBPaxos) (err error) {

	if cluster.Status.Phase == v1alpha1.GreatDBPaxosPending {
		cluster.Status.Phase = v1alpha1.GreatDBPaxosDeployDB
	}

	great.UpdateTargetInstanceToMember(cluster)

	if err = great.CreateOrUpdateGreatDB(cluster); err != nil {
		return err
	}

	if err = great.UpdateGreatDBStatus(cluster); err != nil {
		return err
	}

	if err = great.Scale(cluster); err != nil {
		return err
	}

	if err = great.failover(cluster); err != nil {
		return err
	}

	return nil

}

func (great GreatDBManager) UpdateTargetInstanceToMember(cluster *v1alpha1.GreatDBPaxos) {

	if cluster.Status.Phase != v1alpha1.GreatDBPaxosDeployDB {
		return
	}

	cluster.Status.Instances = cluster.Spec.Instances
	cluster.Status.TargetInstances = cluster.Spec.Instances

	if cluster.Status.Member == nil {
		cluster.Status.Member = make([]v1alpha1.MemberCondition, 0)
	}

	num := len(cluster.Status.Member)

	if num >= int(cluster.Status.TargetInstances) {
		return
	}
	index := GetNextIndex(cluster.Status.Member)

	for num < int(cluster.Status.TargetInstances) {
		name := fmt.Sprintf("%s%s-%d", cluster.Name, resourcemanager.ComponentGreatDBSuffix, index)
		cluster.Status.Member = append(cluster.Status.Member, v1alpha1.MemberCondition{
			Name:       name,
			Index:      index,
			CreateType: v1alpha1.InitCreateMember,
			PvcName:    name,
		})
		num++
		index += 1

	}

}

func (great *GreatDBManager) CreateOrUpdateGreatDB(cluster *v1alpha1.GreatDBPaxos) error {

	for _, member := range cluster.Status.Member {
		if err := great.CreateOrUpdateInstance(cluster, member); err != nil {
			return err
		}
	}

	return nil
}

func (great GreatDBManager) CreateOrUpdateInstance(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) error {

	ns := cluster.GetNamespace()

	if err := great.SyncPvc(cluster, member); err != nil {
		return err
	}

	if cluster.DeletionTimestamp.IsZero() {

		// pause
		pause, err := great.pauseGreatDB(cluster, member)
		if err != nil {
			return err
		}
		// If the instance is paused, end processing
		if pause {
			return nil
		}
	}

	del, err := great.deleteGreatDB(cluster, member)
	if err != nil {
		return err
	}

	if del {
		return nil
	}

	// create
	pod := &corev1.Pod{}
	err = great.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: member.Name}, pod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// create greatdb instance
			if err = great.createGreatDBPod(cluster, member); err != nil {
				return err
			}
			// if member.CreateType == v1alpha1.ScaleCreateMember || member.CreateType == v1alpha1.FailOverCreateMember {
			// 	cluster.Status.CurrentInstances += 1
			// }
			return nil
		}
		dblog.Log.Errorf("failed to sync greatDB pods of cluster %s/%s", ns, cluster.Name)
		return err
	}

	newPod := pod.DeepCopy()

	if cluster.DeletionTimestamp.IsZero() {
		// restart
		if err := great.restartGreatDB(cluster, newPod); err != nil {
			return err
		}

		if _, ok := cluster.Status.RestartMember.Restarting[pod.Name]; ok {
			return nil
		}

		// upgrade
		err = great.upgradeGreatDB(cluster, newPod)
		if err != nil {
			return err
		}

		if _, ok := cluster.Status.UpgradeMember.Upgrading[pod.Name]; ok {
			return nil
		}

	}

	// update meta
	if err = great.updateGreatDBPod(cluster, newPod); err != nil {
		return err
	}

	dblog.Log.Infof("Successfully update the greatdb pods %s/%s", ns, member.Name)
	return nil

}

func (great GreatDBManager) createGreatDBPod(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) error {
	// The cluster starts to clean, and no more resources need to be created
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}

	pod, _ := great.NewGreatDBPod(cluster, member)

	if pod == nil {
		return nil
	}

	_, err := great.Client.KubeClientset.CoreV1().Pods(cluster.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			// The pods already exists, but for unknown reasons, the operator has not monitored the greatdb pod.
			//  need to add a label to the configmap to ensure that the operator can monitor it
			labels, _ := json.Marshal(pod.ObjectMeta.Labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labels)
			_, err = great.Client.KubeClientset.CoreV1().Pods(cluster.Namespace).Patch(
				context.TODO(), pod.Name, types.StrategicMergePatchType, []byte(data), metav1.PatchOptions{})
			if err != nil {
				dblog.Log.Errorf("failed to update the labels of pod, message: %s", err.Error())
				return err
			}
			return nil
		}

		dblog.Log.Errorf("failed to create  greatdb pod %s/%s  , message: %s", pod.Namespace, pod.Name, err.Error())
		return err
	}

	dblog.Log.Infof("Successfully created the greatdb pod %s/%s", pod.Namespace, pod.Name)

	return nil

}

func (great GreatDBManager) NewGreatDBPod(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (pod *corev1.Pod, err error) {

	owner := resourcemanager.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
	labels := resourcemanager.MegerLabels(cluster.Spec.Labels, great.GetLabels(cluster.Name))
	// TODO Debug
	labels[resourcemanager.AppKubePodLabelKey] = member.Name
	podSpec, err := great.GetPodSpec(cluster, member)
	if err != nil {
		return nil, err
	}
	pod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Annotations:     cluster.Spec.Annotations,
			OwnerReferences: []metav1.OwnerReference{owner},
			Finalizers:      []string{resourcemanager.FinalizersGreatDBCluster},
			Name:            member.Name,
			Namespace:       cluster.Namespace,
		},
		Spec: podSpec,
	}

	return
}

// GetLabels  Return to the default label settings
func (great GreatDBManager) GetLabels(clusterName string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resourcemanager.AppKubeNameLabelKey] = resourcemanager.AppKubeNameLabelValue
	labels[resourcemanager.AppkubeManagedByLabelKey] = resourcemanager.AppkubeManagedByLabelValue
	labels[resourcemanager.AppKubeInstanceLabelKey] = clusterName
	labels[resourcemanager.AppKubeComponentLabelKey] = resourcemanager.AppKubeComponentGreatDB

	return

}

func (great GreatDBManager) GetPodSpec(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (podSpec corev1.PodSpec, err error) {

	configmapName := cluster.Name + resourcemanager.ComponentGreatDBSuffix
	serviceName := cluster.Name + resourcemanager.ComponentGreatDBSuffix

	containers := great.newContainers(serviceName, cluster, member)
	initContainer, err := great.newGreatDBInitContainers(cluster, member)
	if err != nil {
		return podSpec, err
	}
	volume := great.newVolumes(configmapName, member.PvcName, cluster)
	// update Affinity
	affinity := cluster.Spec.Affinity

	if affinity == nil {
		affinity = getDefaultAffinity(cluster.Name)
	}

	podSpec = corev1.PodSpec{
		InitContainers:    initContainer,
		Containers:        containers,
		Volumes:           volume,
		PriorityClassName: cluster.Spec.PriorityClassName,
		SecurityContext:   &cluster.Spec.PodSecurityContext,
		ImagePullSecrets:  cluster.Spec.ImagePullSecrets,
		Affinity:          affinity,
		Hostname:          member.Name,
		Subdomain:         serviceName,
	}

	return

}

// newGreatDBInitContainers
func (great GreatDBManager) newGreatDBInitContainers(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (containers []corev1.Container, err error) {
	var lastBackuprecord *v1alpha1.GreatDBBackupRecord
	if member.CreateType == v1alpha1.ScaleCreateMember || member.CreateType == v1alpha1.FailOverCreateMember {

		if cluster.Spec.Scaling.ScaleOut.Source == v1alpha1.ScaleOutSourceBackup {
			lastBackuprecord = great.getLatestSuccessfulBackup(cluster.Namespace, cluster.Name, string(v1alpha1.GreatDBBackupResourceType))
		}

		if lastBackuprecord == nil {
			instanceName := make([]string, 0)
			primaryIns := ""
			ins := ""

			for _, member := range cluster.Status.Member {

				if member.Type != v1alpha1.MemberStatusOnline {
					continue
				}
				if v1alpha1.MemberRoleType(member.Role).Parse() == v1alpha1.MemberRolePrimary {
					primaryIns = member.Name
				} else {
					instanceName = append(instanceName, member.Name)
				}

			}

			if len(instanceName) > 0 {
				if len(instanceName) == 1 {
					ins = instanceName[0]
				} else {
					rand.Seed(time.Now().UnixNano())
					ins = instanceName[rand.Intn(len(instanceName))]
				}
			}

			if primaryIns != "" && ins == "" {
				ins = primaryIns
			}
			if ins == "" {
				return nil, fmt.Errorf("No ready instances available for cloning operation")
			}

			db := great.newGreatDBCloneContainers(ins, cluster)
			containers = append(containers, db)

		} else {
			db := great.newGreatDBBackupContainers(lastBackuprecord, cluster)
			containers = append(containers, db)
		}

	}

	if member.CreateType == v1alpha1.InitCreateMember && cluster.Spec.CloneSource != nil {

		if cluster.Spec.CloneSource.ClusterName == "" && cluster.Spec.CloneSource.BackupRecordName == "" {
			return containers, nil
		}

		ns := cluster.Spec.CloneSource.Namespace
		if ns == "" {
			ns = cluster.Namespace
		}
		cluster.Status.CloneSource.Namespace = ns

		clusterName := cluster.Status.CloneSource.ClusterName
		if clusterName == "" {
			clusterName = cluster.Spec.CloneSource.ClusterName
		}

		recordName := cluster.Status.CloneSource.BackupRecordName
		if recordName == "" {
			recordName = cluster.Spec.CloneSource.BackupRecordName
		}

		if recordName == "" {

			lastBackuprecord = great.getLatestSuccessfulBackup(ns, clusterName, string(v1alpha1.GreatDBBackupResourceType))
			if lastBackuprecord == nil {
				return containers, fmt.Errorf("the clone source does not have a completed backup record")
			}
			cluster.Status.CloneSource.ClusterName = clusterName
		} else {
			lastBackuprecord, err = great.Lister.BackupRecordLister.GreatDBBackupRecords(ns).Get(recordName)
			if err != nil {
				dblog.Log.Reason(err).Error("failed to get GreatDBBackupRecords")
				return containers, err
			}
			cluster.Status.CloneSource.ClusterName = clusterName
		}

		cluster.Status.CloneSource.BackupRecordName = lastBackuprecord.Name
		db := great.newGreatDBBackupContainers(lastBackuprecord, cluster)
		containers = append(containers, db)

	}

	return
}

func (great GreatDBManager) newContainers(serviceName string, cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (containers []corev1.Container) {

	// greatdb
	db := great.newGreatDBContainers(serviceName, cluster)

	containers = append(containers, db)

	// greatdb-agent
	if *cluster.Spec.Backup.Enable {
		backup := great.newGreatDBAgentContainers(serviceName, cluster)
		containers = append(containers, backup)
	}

	//  Add exporter sidecar container
	if cluster.Spec.LogCollection.Image != "" {
		promtail := great.newPromtailContainers(serviceName, cluster)
		containers = append(containers, promtail)
	}

	// Add another container

	return
}

func (great GreatDBManager) newGreatDBContainers(serviceName string, cluster *v1alpha1.GreatDBPaxos) (container corev1.Container) {

	env := great.newGreatDBEnv(serviceName, cluster)
	envForm := great.newGreatDBEnvForm(cluster.Spec.SecretName)
	imagePullPolicy := corev1.PullIfNotPresent
	if cluster.Spec.ImagePullPolicy != "" {
		imagePullPolicy = cluster.Spec.ImagePullPolicy
	}

	container = corev1.Container{
		Name:            GreatDBContainerName,
		EnvFrom:         envForm,
		Env:             env,
		Command:         []string{"start-greatdb.sh"},
		Image:           cluster.Spec.Image,
		Resources:       cluster.Spec.Resources,
		ImagePullPolicy: imagePullPolicy,
		VolumeMounts: []corev1.VolumeMount{
			{ // config
				Name:      resourcemanager.GreatDBPvcConfigName,
				MountPath: greatdbConfigMountPath,
				ReadOnly:  true,
			},
			{ // data
				Name:      resourcemanager.GreatdbPvcDataName,
				MountPath: greatdbDataMountPath,
			},
		},
		ReadinessProbe: &corev1.Probe{
			PeriodSeconds:       5,
			InitialDelaySeconds: 30,
			FailureThreshold:    3,
			TimeoutSeconds:      2,
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{Command: []string{"ReadinessProbe"}},
			},
		},
		LivenessProbe: &corev1.Probe{
			PeriodSeconds:       10,
			InitialDelaySeconds: 30,
			FailureThreshold:    3,
			TimeoutSeconds:      3,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.FromInt(8001),
				},
			},
		},
	}

	if config.ServerVersion >= "v1.17.0" {
		container.StartupProbe = &corev1.Probe{
			PeriodSeconds:    30,
			FailureThreshold: 20,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(int(cluster.Spec.Port))},
			},
			TimeoutSeconds: 2,
		}
	}
	return
}

func (great GreatDBManager) newGreatDBEnv(serviceName string, cluster *v1alpha1.GreatDBPaxos) (env []corev1.EnvVar) {

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
			Name:  "CLUSTERDOMAIN",
			Value: cluster.GetClusterDomain(),
		},
		{
			Name:  "FQDN",
			Value: "$(PODNAME).$(SERVICE_NAME).$(NAMESPACE).svc.$(CLUSTERDOMAIN)",
		},
		{
			Name:  "SERVERPORT",
			Value: fmt.Sprintf("%d", cluster.Spec.Port),
		}, //
		{
			Name:  "GROUPLOCALADDRESS",
			Value: "$(PODNAME).$(SERVICE_NAME).$(NAMESPACE).svc.$(CLUSTERDOMAIN)" + fmt.Sprintf(":%d", resourcemanager.GroupPort),
		},
	}

	return
}

func (great GreatDBManager) newGreatDBEnvForm(secretName string) (env []corev1.EnvFromSource) {
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

func (great GreatDBManager) newVolumes(configmapName, pvcName string, cluster *v1alpha1.GreatDBPaxos) (volumes []corev1.Volume) {

	volumes = []corev1.Volume{}

	if configmapName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: resourcemanager.GreatDBPvcConfigName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: configmapName},
				},
			},
		})

	}

	if pvcName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: resourcemanager.GreatdbPvcDataName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		})
	}

	if *cluster.Spec.Backup.Enable {

		if cluster.Spec.Backup.NFS != nil && cluster.Spec.Backup.NFS.Path != "" && cluster.Spec.Backup.NFS.Server != "" {
			volumes = append(volumes, corev1.Volume{
				Name: resourcemanager.GreatdbBackupPvcName,
				VolumeSource: corev1.VolumeSource{
					NFS: &corev1.NFSVolumeSource{
						Path:   cluster.Spec.Backup.NFS.Path,
						Server: cluster.Spec.Backup.NFS.Server,
					},
				},
			})

		}

	}

	return

}

func (great GreatDBManager) newGreatDBAgentContainers(serviceName string, cluster *v1alpha1.GreatDBPaxos) (container corev1.Container) {

	env := great.newGreatAgentDBEnv(serviceName, cluster)
	envForm := great.newGreatDBEnvForm(cluster.Spec.SecretName)
	imagePullPolicy := corev1.PullIfNotPresent
	if cluster.Spec.ImagePullPolicy != "" {
		imagePullPolicy = cluster.Spec.ImagePullPolicy
	}

	volumeMounts := []corev1.VolumeMount{

		{ // data
			Name:      resourcemanager.GreatdbPvcDataName,
			MountPath: greatdbDataMountPath,
		},
	}
	if cluster.Spec.Backup.NFS != nil {
		backupMounts := corev1.VolumeMount{
			Name:      resourcemanager.GreatdbBackupPvcName,
			MountPath: greatdbBackupMountPath,
		}
		volumeMounts = append(volumeMounts, backupMounts)
	}

	container = corev1.Container{
		Name:            GreatDBAgentContainerName,
		EnvFrom:         envForm,
		Env:             env,
		Image:           cluster.Spec.Image,
		ImagePullPolicy: imagePullPolicy,
		Resources:       cluster.Spec.Backup.Resources,
		Command:         []string{"greatdb-agent", "--mode=server"},
		VolumeMounts:    volumeMounts,
		ReadinessProbe: &corev1.Probe{
			PeriodSeconds:       10,
			InitialDelaySeconds: 30,
			FailureThreshold:    3,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.FromInt(BackupServerPort),
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: 15,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(BackupServerPort)},
			},
		},
	}

	return
}

func (great GreatDBManager) newGreatAgentDBEnv(serviceName string, cluster *v1alpha1.GreatDBPaxos) (env []corev1.EnvVar) {

	env = []corev1.EnvVar{
		{
			Name: "HOSTNAME",
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
			Name:  "ClusterName",
			Value: cluster.Name,
		},
		{
			Name:  "BackupHost",
			Value: "$(HOSTNAME).$(SERVICE_NAME).$(NAMESPACE).svc." + cluster.GetClusterDomain(),
		},
		{
			Name:  "BackupPort",
			Value: strconv.Itoa(int(cluster.Spec.Port)),
		},
	}

	return
}

func (great GreatDBManager) newPromtailContainers(serviceName string, cluster *v1alpha1.GreatDBPaxos) (container corev1.Container) {
	imagePullPolicy := corev1.PullIfNotPresent
	if cluster.Spec.ImagePullPolicy != "" {
		imagePullPolicy = cluster.Spec.ImagePullPolicy
	}
	lokiClient := cluster.Spec.LogCollection.LokiClient
	if lokiClient == "" {
		svcName := cluster.Name + resourcemanager.ComponentDashboardSuffix
		dashboardUriStr := "http://%s:8080/log-monitor/loki/api/v1/push"
		lokiClient = fmt.Sprintf(dashboardUriStr, svcName)
	}
	env := great.newGreatDBEnv(serviceName, cluster)
	envForm := great.newGreatDBEnvForm(cluster.Spec.SecretName)
	promtailEnv := []corev1.EnvVar{
		{
			Name:  "INSTANCE_INTERIOR_PORT",
			Value: fmt.Sprintf("%d", cluster.Spec.Port),
		},
		{
			Name:  "LOKI_CLIENT",
			Value: lokiClient,
		},
	}
	env = append(env, promtailEnv...)
	container = corev1.Container{
		Name:            "promtail",
		Image:           cluster.Spec.LogCollection.Image,
		ImagePullPolicy: imagePullPolicy,
		Resources:       cluster.Spec.LogCollection.Resources,
		Command:         []string{"/usr/bin/promtail", "-config.expand-env=true", "-config.file=/etc/promtail/promtail.yaml"},
		Env:             env,
		EnvFrom:         envForm,
		VolumeMounts: []corev1.VolumeMount{
			{ // data
				Name:      resourcemanager.GreatdbPvcDataName,
				MountPath: greatdbDataMountPath,
			},
		},
		ReadinessProbe: &corev1.Probe{
			PeriodSeconds:       10,
			InitialDelaySeconds: 10,
			FailureThreshold:    3,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(3001)},
			},
		},
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: 10,
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(3001)},
			},
		},
	}

	return
}

// updateGreatDBStatefulSet  Update greatdb statefulset
func (great GreatDBManager) updateGreatDBPod(cluster *v1alpha1.GreatDBPaxos, podIns *corev1.Pod) error {

	if !cluster.DeletionTimestamp.IsZero() {
		if len(podIns.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
		_, err := great.Client.KubeClientset.CoreV1().Pods(podIns.Namespace).Patch(
			context.TODO(), podIns.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of pods  %s/%s,message: %s", podIns.Namespace, podIns.Name, err.Error())
		}

		return nil

	}
	needUpdate := false
	// update labels
	if up := great.updateLabels(podIns, cluster); up {
		needUpdate = true
	}

	// update annotations
	if up := great.updateAnnotations(podIns, cluster); up {
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

		err := great.updatePod(podIns)
		if err != nil {
			return err
		}

	}

	return nil

}

func (great GreatDBManager) updatePod(pod *corev1.Pod) error {

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := great.Client.KubeClientset.CoreV1().Pods(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}
		upPod, err1 := great.Lister.PodLister.Pods(pod.Namespace).Get(pod.Name)
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

func (great GreatDBManager) updateLabels(pod *corev1.Pod, cluster *v1alpha1.GreatDBPaxos) bool {
	needUpdate := false
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	labels := resourcemanager.MegerLabels(cluster.Spec.Labels, great.GetLabels(cluster.Name))
	for key, value := range labels {
		if v, ok := pod.Labels[key]; !ok || v != value {
			pod.Labels[key] = value
			needUpdate = true
		}
	}

	return needUpdate
}

func (great GreatDBManager) updateAnnotations(pod *corev1.Pod, cluster *v1alpha1.GreatDBPaxos) bool {
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

func (great GreatDBManager) UpdateGreatDBStatus(cluster *v1alpha1.GreatDBPaxos) error {
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}
	var diag ClusterStatus
	if cluster.Status.Phase.Stage() > 3 {
		diag = great.probeStatusIfNeeded(cluster)
	} else {
		diag = great.ProbeStatus(cluster)
	}

	switch cluster.Status.Phase {

	case v1alpha1.GreatDBPaxosDeployDB:

		cluster.Status.Port = cluster.Spec.Port
		cluster.Status.Instances = cluster.Spec.Instances
		cluster.Status.TargetInstances = cluster.Spec.Instances

		if !diag.checkInstanceContainersIsReady() {
			return nil
		}
		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosBootCluster, "")
		return nil

	case v1alpha1.GreatDBPaxosBootCluster:

		if err := diag.createCluster(cluster); err != nil {
			return err
		}
		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosInitUser, "")
		return nil

	case v1alpha1.GreatDBPaxosInitUser:
		if err := diag.initUser(cluster); err != nil {
			return err
		}
		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		return nil

	case v1alpha1.GreatDBPaxosReady:
		// pause
		if cluster.Spec.Pause.Enable && cluster.Spec.Pause.Mode == v1alpha1.ClusterPause {
			msg := fmt.Sprintf("The cluster diagnosis status is %s", diag.status)
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosPause, msg)
			break
		}

		if cluster.Status.ReadyInstances == 0 {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosRepair, "")
		}

	case v1alpha1.GreatDBPaxosPause:

		if (cluster.Spec.Pause.Enable && cluster.Spec.Pause.Mode != v1alpha1.ClusterPause || !cluster.Spec.Pause.Enable) && cluster.Status.ReadyInstances > 0 {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		}

	case v1alpha1.GreatDBPaxosRestart:

		if !cluster.Spec.Restart.Enable && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		}

	case v1alpha1.GreatDBPaxosUpgrade:
		if len(cluster.Status.UpgradeMember.Upgrading) == 0 && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "upgrade successful")
		}

	case v1alpha1.GreatDBPaxosRepair:
		if cluster.Status.ReadyInstances < cluster.Spec.Instances {
			diag.repairCluster(cluster)
		}
		if cluster.Status.ReadyInstances == cluster.Spec.Instances {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		}

		return nil
	case v1alpha1.GreatDBPaxosScaleOut:
		if cluster.Status.TargetInstances == cluster.Status.CurrentInstances && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		}
	case v1alpha1.GreatDBPaxosScaleIn:
		if cluster.Status.TargetInstances == cluster.Status.CurrentInstances && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		}

	case v1alpha1.GreatDBPaxosFailover:
		if cluster.Status.TargetInstances == cluster.Status.CurrentInstances && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosFailoverSucceeded, "")
		}

	case v1alpha1.GreatDBPaxosFailoverSucceeded:
		if cluster.Status.TargetInstances == cluster.Spec.Instances {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
			break
		}
		diag.repairCluster(cluster)
		return nil
	}

	if cluster.Status.ReadyInstances < cluster.Status.Instances {
		diag.repairCluster(cluster)
	}

	return nil
}

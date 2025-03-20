package core

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"greatdb.com/greatdb-operator/api/v1"
	"greatdb.com/greatdb-operator/internal/config"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager/greatdbpaxos/configmap"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager/secret"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager/service"
	dblog "greatdb.com/greatdb-operator/internal/utils/log"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GreatDBPaxosManager struct {
	Client   client.Client
	Recorder record.EventRecorder
}

type Manager interface {
	Sync(ctx context.Context, cluster *v1.GreatDBPaxos) (err error)
}

type GreatDBPaxosResourceManagers struct {
	ConfigMapManager    Manager
	ServiceManager      Manager
	SecretManager       Manager
	GreatDBPaxosManager Manager
	DashboardManager    Manager
}

func NewGreatDBPaxosResourceManagers(client client.Client, recorder record.EventRecorder) *GreatDBPaxosResourceManagers {
	configmapManager := &configmap.Manager{Client: client, Recorder: recorder}
	serviceManager := &service.Manager{Client: client, Recorder: recorder}
	secretmanager := &secret.Manager{Client: client, Recorder: recorder}
	dashboardManager := &DashboardManager{Client: client, Recorder: recorder}
	greatdbPaxosManager := &GreatDBPaxosManager{Client: client, Recorder: recorder}

	return &GreatDBPaxosResourceManagers{
		ConfigMapManager:    configmapManager,
		ServiceManager:      serviceManager,
		SecretManager:       secretmanager,
		GreatDBPaxosManager: greatdbPaxosManager,
		DashboardManager:    dashboardManager,
	}

}

func (g *GreatDBPaxosManager) Sync(ctx context.Context, cluster *v1.GreatDBPaxos) (err error) {

	if cluster.Status.Phase == v1.GreatDBPaxosPending {
		cluster.Status.Phase = v1.GreatDBPaxosDeployDB
	}

	g.UpdateTargetInstanceToMember(cluster)

	if err = g.CreateOrUpdateGreatDB(ctx, cluster); err != nil {
		return err
	}

	if err = g.UpdateGreatDBStatus(ctx, cluster); err != nil {
		return err
	}

	if err = g.Scale(ctx, cluster); err != nil {
		return err
	}

	if err = g.failOver(ctx, cluster); err != nil {
		return err
	}

	return nil

}

func (g *GreatDBPaxosManager) UpdateTargetInstanceToMember(cluster *v1.GreatDBPaxos) {

	if cluster.Status.Phase != v1.GreatDBPaxosDeployDB {
		return
	}

	cluster.Status.Instances = cluster.Spec.Instances
	cluster.Status.TargetInstances = cluster.Spec.Instances

	if cluster.Status.Member == nil {
		cluster.Status.Member = make([]v1.MemberCondition, 0)
	}

	num := len(cluster.Status.Member)

	if num >= int(cluster.Status.TargetInstances) {
		return
	}
	index := GetNextIndex(cluster.Status.Member)

	for num < int(cluster.Status.TargetInstances) {
		name := fmt.Sprintf("%s%s-%d", cluster.Name, resourcemanager.ComponentGreatDBSuffix, index)
		cluster.Status.Member = append(cluster.Status.Member, v1.MemberCondition{
			Name:       name,
			Index:      index,
			CreateType: v1.InitCreateMember,
			PvcName:    name,
		})
		num++
		index += 1

	}

}

func (g *GreatDBPaxosManager) CreateOrUpdateGreatDB(ctx context.Context, cluster *v1.GreatDBPaxos) error {

	for _, member := range cluster.Status.Member {
		if err := g.CreateOrUpdateInstance(ctx, cluster, member); err != nil {
			return err
		}
	}

	return nil
}

func (g *GreatDBPaxosManager) CreateOrUpdateInstance(ctx context.Context, cluster *v1.GreatDBPaxos, member v1.MemberCondition) error {

	ns := cluster.GetNamespace()

	if err := g.SyncPvc(ctx, cluster, member); err != nil {
		return err
	}

	if cluster.DeletionTimestamp.IsZero() {

		// pause
		pause, err := g.pauseGreatDB(ctx, cluster, member)
		if err != nil {
			return err
		}
		// If the instance is paused, end processing
		if pause {
			return nil
		}
	}

	del, err := g.deleteGreatDB(ctx, cluster, member)
	if err != nil {
		return err
	}

	if del {
		return nil
	}

	// create
	pod := &corev1.Pod{}
	err = g.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: member.Name}, pod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// create greatdb instance
			if err = g.createGreatDBPod(ctx, cluster, member); err != nil {
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
		if err := g.restartGreatDB(ctx, cluster, newPod); err != nil {
			return err
		}

		if _, ok := cluster.Status.RestartMember.Restarting[pod.Name]; ok {
			return nil
		}

		// upgrade
		err = g.upgradeGreatDB(ctx, cluster, newPod)
		if err != nil {
			return err
		}

		if _, ok := cluster.Status.UpgradeMember.Upgrading[pod.Name]; ok {
			return nil
		}

	}

	// update meta
	if err = g.updateGreatDBPod(ctx, cluster, newPod); err != nil {
		return err
	}

	dblog.Log.Infof("Successfully update the greatdb pods %s/%s", ns, member.Name)
	return nil

}

func (g *GreatDBPaxosManager) createGreatDBPod(ctx context.Context, cluster *v1.GreatDBPaxos, member v1.MemberCondition) error {
	// The cluster starts to clean, and no more resources need to be created
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}

	pod, _ := g.NewGreatDBPod(ctx, cluster, member)

	if pod == nil {
		return nil
	}

	err := g.Client.Create(ctx, pod, &client.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			// The pods already exists, but for unknown reasons, the operator has not monitored the greatdb pod.
			//  need to add a label to the configmap to ensure that the operator can monitor it
			labels, _ := json.Marshal(pod.ObjectMeta.Labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labels)
			patch := client.RawPatch(types.StrategicMergePatchType, []byte(data))
			err = g.Client.Patch(ctx, pod, patch)
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

func (g *GreatDBPaxosManager) NewGreatDBPod(ctx context.Context, cluster *v1.GreatDBPaxos, member v1.MemberCondition) (pod *corev1.Pod, err error) {

	owner := resourcemanager.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
	labels := resourcemanager.MergeLabels(cluster.Spec.Labels, g.GetLabels(cluster.Name))
	// TODO Debug
	labels[resourcemanager.AppKubePodLabelKey] = member.Name
	podSpec, err := g.GetPodSpec(ctx, cluster, member)
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
func (g *GreatDBPaxosManager) GetLabels(clusterName string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resourcemanager.AppKubeNameLabelKey] = resourcemanager.AppKubeNameLabelValue
	labels[resourcemanager.AppkubeManagedByLabelKey] = resourcemanager.AppkubeManagedByLabelValue
	labels[resourcemanager.AppKubeInstanceLabelKey] = clusterName
	labels[resourcemanager.AppKubeComponentLabelKey] = resourcemanager.AppKubeComponentGreatDB

	return

}

func (g *GreatDBPaxosManager) GetPodSpec(ctx context.Context, cluster *v1.GreatDBPaxos, member v1.MemberCondition) (podSpec corev1.PodSpec, err error) {

	configmapName := cluster.Name + resourcemanager.ComponentGreatDBSuffix
	serviceName := cluster.Name + resourcemanager.ComponentGreatDBSuffix

	containers := g.newContainers(serviceName, cluster, member)
	initContainer, err := g.newGreatDBInitContainers(ctx, cluster, member)
	if err != nil {
		return podSpec, err
	}
	volume := g.newVolumes(configmapName, member.PvcName, cluster)
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
func (g *GreatDBPaxosManager) newGreatDBInitContainers(ctx context.Context, cluster *v1.GreatDBPaxos, member v1.MemberCondition) (containers []corev1.Container, err error) {
	var lastBackuprecord *v1.GreatDBBackupRecord
	if member.CreateType == v1.ScaleCreateMember || member.CreateType == v1.FailOverCreateMember {

		if cluster.Spec.Scaling.ScaleOut.Source == v1.ScaleOutSourceBackup {
			lastBackuprecord = g.getLatestSuccessfulBackup(ctx, cluster.Namespace, cluster.Name, string(v1.GreatDBBackupResourceType))
		}

		if lastBackuprecord == nil {
			instanceName := make([]string, 0)
			primaryIns := ""
			ins := ""

			for _, member := range cluster.Status.Member {

				if member.Type != v1.MemberStatusOnline {
					continue
				}
				if v1.MemberRoleType(member.Role).Parse() == v1.MemberRolePrimary {
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

			db := g.newGreatDBCloneContainers(ins, cluster)
			containers = append(containers, db)

		} else {
			db := g.newGreatDBBackupContainers(lastBackuprecord, cluster)
			containers = append(containers, db)
		}

	}

	if member.CreateType == v1.InitCreateMember && cluster.Spec.CloneSource != nil {

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

			lastBackuprecord = g.getLatestSuccessfulBackup(ctx, ns, clusterName, string(v1.GreatDBBackupResourceType))
			if lastBackuprecord == nil {
				return containers, fmt.Errorf("the clone source does not have a completed backup record")
			}
			cluster.Status.CloneSource.ClusterName = clusterName
		} else {
			err = g.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: recordName}, lastBackuprecord)
			if err != nil {
				dblog.Log.Reason(err).Error("failed to get GreatDBBackupRecords")
				return containers, err
			}
			cluster.Status.CloneSource.ClusterName = clusterName
		}

		cluster.Status.CloneSource.BackupRecordName = lastBackuprecord.Name
		db := g.newGreatDBBackupContainers(lastBackuprecord, cluster)
		containers = append(containers, db)

	}

	return
}

func (g *GreatDBPaxosManager) newContainers(serviceName string, cluster *v1.GreatDBPaxos, member v1.MemberCondition) (containers []corev1.Container) {

	// greatdb
	db := g.newGreatDBContainers(serviceName, cluster)

	containers = append(containers, db)

	// greatdb-agent
	if *cluster.Spec.Backup.Enable {
		backup := g.newGreatDBAgentContainers(serviceName, cluster)
		containers = append(containers, backup)
	}

	//  Add exporter sidecar container
	if cluster.Spec.LogCollection.Image != "" {
		promtail := g.newPromtailContainers(serviceName, cluster)
		containers = append(containers, promtail)
	}

	// Add another container

	return
}

func (g *GreatDBPaxosManager) newGreatDBContainers(serviceName string, cluster *v1.GreatDBPaxos) (container corev1.Container) {

	env := g.newGreatDBEnv(serviceName, cluster)
	envForm := g.newGreatDBEnvForm(cluster.Spec.SecretName)
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

func (g *GreatDBPaxosManager) newGreatDBEnv(serviceName string, cluster *v1.GreatDBPaxos) (env []corev1.EnvVar) {

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

func (g *GreatDBPaxosManager) newGreatDBEnvForm(secretName string) (env []corev1.EnvFromSource) {
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

func (g *GreatDBPaxosManager) newVolumes(configmapName, pvcName string, cluster *v1.GreatDBPaxos) (volumes []corev1.Volume) {

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

func (g *GreatDBPaxosManager) newGreatDBAgentContainers(serviceName string, cluster *v1.GreatDBPaxos) (container corev1.Container) {

	env := g.newGreatAgentDBEnv(serviceName, cluster)
	envForm := g.newGreatDBEnvForm(cluster.Spec.SecretName)
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

func (g *GreatDBPaxosManager) newGreatAgentDBEnv(serviceName string, cluster *v1.GreatDBPaxos) (env []corev1.EnvVar) {

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

func (g *GreatDBPaxosManager) newPromtailContainers(serviceName string, cluster *v1.GreatDBPaxos) (container corev1.Container) {
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
	env := g.newGreatDBEnv(serviceName, cluster)
	envForm := g.newGreatDBEnvForm(cluster.Spec.SecretName)
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
func (g *GreatDBPaxosManager) updateGreatDBPod(ctx context.Context, cluster *v1.GreatDBPaxos, podIns *corev1.Pod) error {

	if !cluster.DeletionTimestamp.IsZero() {
		if len(podIns.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
		err := g.Client.Patch(ctx, podIns, client.RawPatch(types.JSONPatchType, []byte(patch)))
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of pods  %s/%s,message: %s", podIns.Namespace, podIns.Name, err.Error())
		}

		return nil

	}
	needUpdate := false
	// update labels
	if up := g.updateLabels(podIns, cluster); up {
		needUpdate = true
	}

	// update annotations
	if up := g.updateAnnotations(podIns, cluster); up {
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

		err := g.updatePod(ctx, podIns)
		if err != nil {
			return err
		}

	}

	return nil

}

func (g *GreatDBPaxosManager) updatePod(ctx context.Context, pod *corev1.Pod) error {

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := g.Client.Update(ctx, pod)
		if err == nil {
			return nil
		}

		upPod := &corev1.Pod{}
		err = g.Client.Get(ctx, client.ObjectKeyFromObject(pod), upPod)
		if err != nil {
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

func (g *GreatDBPaxosManager) updateLabels(pod *corev1.Pod, cluster *v1.GreatDBPaxos) bool {
	needUpdate := false
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	labels := resourcemanager.MergeLabels(cluster.Spec.Labels, g.GetLabels(cluster.Name))
	for key, value := range labels {
		if v, ok := pod.Labels[key]; !ok || v != value {
			pod.Labels[key] = value
			needUpdate = true
		}
	}

	return needUpdate
}

func (g *GreatDBPaxosManager) updateAnnotations(pod *corev1.Pod, cluster *v1.GreatDBPaxos) bool {
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

func (g *GreatDBPaxosManager) UpdateGreatDBStatus(ctx context.Context, cluster *v1.GreatDBPaxos) error {
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}
	var diag ClusterStatus
	if cluster.Status.Phase.Stage() > 3 {
		diag = g.probeStatusIfNeeded(ctx, cluster)
	} else {
		diag = g.ProbeStatus(ctx, cluster)
	}

	switch cluster.Status.Phase {

	case v1.GreatDBPaxosDeployDB:

		cluster.Status.Port = cluster.Spec.Port
		cluster.Status.Instances = cluster.Spec.Instances
		cluster.Status.TargetInstances = cluster.Spec.Instances

		if !diag.checkInstanceContainersIsReady() {
			return nil
		}
		UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosBootCluster, "")
		return nil

	case v1.GreatDBPaxosBootCluster:

		if err := diag.createCluster(cluster); err != nil {
			return err
		}
		UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosInitUser, "")
		return nil

	case v1.GreatDBPaxosInitUser:
		if err := diag.initUser(cluster); err != nil {
			return err
		}
		UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosReady, "")
		return nil

	case v1.GreatDBPaxosReady:
		// pause
		if cluster.Spec.Pause.Enable && cluster.Spec.Pause.Mode == v1.ClusterPause {
			msg := fmt.Sprintf("The cluster diagnosis status is %s", diag.status)
			UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosPause, msg)
			break
		}

		if cluster.Status.ReadyInstances == 0 {
			UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosRepair, "")
		}

	case v1.GreatDBPaxosPause:

		if (cluster.Spec.Pause.Enable && cluster.Spec.Pause.Mode != v1.ClusterPause || !cluster.Spec.Pause.Enable) && cluster.Status.ReadyInstances > 0 {
			UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosReady, "")
		}

	case v1.GreatDBPaxosRestart:

		if !cluster.Spec.Restart.Enable && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosReady, "")
		}

	case v1.GreatDBPaxosUpgrade:
		if len(cluster.Status.UpgradeMember.Upgrading) == 0 && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosReady, "upgrade successful")
		}

	case v1.GreatDBPaxosRepair:
		if cluster.Status.ReadyInstances < cluster.Spec.Instances {
			diag.repairCluster(cluster)
		}
		if cluster.Status.ReadyInstances == cluster.Spec.Instances {
			UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosReady, "")
		}

		return nil
	case v1.GreatDBPaxosScaleOut:
		if cluster.Status.TargetInstances == cluster.Status.CurrentInstances && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosReady, "")
		}
	case v1.GreatDBPaxosScaleIn:
		if cluster.Status.TargetInstances == cluster.Status.CurrentInstances && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosReady, "")
		}

	case v1.GreatDBPaxosFailover:
		if cluster.Status.TargetInstances == cluster.Status.CurrentInstances && cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosFailoverSucceeded, "")
		}

	case v1.GreatDBPaxosFailoverSucceeded:
		if cluster.Status.TargetInstances == cluster.Spec.Instances {
			UpdateClusterStatusCondition(cluster, v1.GreatDBPaxosReady, "")
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

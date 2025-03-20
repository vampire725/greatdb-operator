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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type User struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Perm     string `json:"perm,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type ServiceType struct {
	// +kubebuilder:default=ClusterIP
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	Type corev1.ServiceType `json:"type"`

	// +optional
	ReadPort int32 `json:"readPort,omitempty"`

	// +optional
	WritePort int32 `json:"writePort,omitempty"`
}

type PauseGreatDB struct {
	// +optional
	Enable bool `json:"enable,omitempty"`

	// +kubebuilder:default=ins
	Mode PauseModeType `json:"mode,omitempty"`

	// +optional
	Instances []string `json:"instances,omitempty"`
}

type RestartGreatDB struct {
	// +optional
	Enable bool `json:"enable,omitempty"`

	// +kubebuilder:default=ins
	Mode RestartModeType `json:"mode,omitempty"`

	// +optional
	Instances []string `json:"instances,omitempty"`

	// +kubebuilder:default=rolling
	Strategy RestartStrategyType `json:"strategy,omitempty"`
}

type DeleteInstance struct {
	// +optional
	Instances []string `json:"instances,omitempty"`

	// +optional
	CleanPvc bool `json:"cleanPvc,omitempty"`
}

type BackupSpec struct {
	// +optional
	// +kubebuilder:default=true
	Enable *bool `json:"enable,omitempty"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	NFS *corev1.NFSVolumeSource `json:"nfs,omitempty"`
}

type Scaling struct {
	// +optional
	ScaleIn ScaleIn `json:"scaleIn,omitempty"`

	// +optional
	ScaleOut ScaleOut `json:"scaleOut,omitempty"`
}

type ScaleIn struct {
	// +kubebuilder:default=fault
	// +kubebuilder:validation:Enum=index;fault;define
	Strategy ScaleInStrategyType `json:"strategy,omitempty"`

	// +optional
	Instance []string `json:"instance,omitempty"`
}

type ScaleOut struct {
	// +kubebuilder:default=backup
	// +kubebuilder:validation:Enum=backup;clone
	// +optional
	Source ScaleOutSourceType `json:"source,omitempty"`
}

type FailOver struct {
	// +kubebuilder:default=true
	Enable *bool `json:"enable,omitempty"`

	// +kubebuilder:default=3
	// +optional
	MaxInstance int32 `json:"maxInstance,omitempty"`

	// +kubebuilder:default="10m"
	// +optional
	Period string `json:"period,omitempty"`

	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// +kubebuilder:default=false
	AutoScaleIn *bool `json:"autoScaleIn,omitempty"`
}

type CloneSource struct {
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// +optional
	BackupRecordName string `json:"backupRecordName,omitempty"`
}

type GreatDBDashboard struct {
	Enable bool `json:"enable"`

	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	PersistentVolumeClaimSpec corev1.PersistentVolumeClaimSpec `json:"persistentVolumeClaimSpec,omitempty"`

	// +optional
	Config map[string]string `json:"config,omitempty"`

	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

type LogCollection struct {
	Image string `json:"image"`

	// +optional
	LokiClient string `json:"lokiClient,omitempty"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// GreatDBPaxosSpec 核心修改点
// +kubebuilder:validation:XPreserveUnknownFields
type GreatDBPaxosSpec struct {
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// +kubebuilder:default=Retain
	// +kubebuilder:validation:Enum=Delete;Retain
	PvReclaimPolicy corev1.PersistentVolumeReclaimPolicy `json:"pvReclaimPolicy,omitempty"`

	// +optional
	PodSecurityContext corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`

	// +optional
	SecretName string `json:"secretName,omitempty"`

	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// +kubebuilder:default=Always
	// +kubebuilder:validation:Enum=Always;IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	MaintenanceMode bool `json:"maintenanceMode"`

	// +kubebuilder:default=cluster.local
	// +optional
	ClusterDomain string `json:"clusterDomain,omitempty"`

	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=2
	// +optional
	Instances int32 `json:"instances,omitempty"`

	// +kubebuilder:validation:Required
	Image string `json:"image"`

	Version string `json:"version"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	VolumeClaimTemplates corev1.PersistentVolumeClaimSpec `json:"volumeClaimTemplates,omitempty"`

	// +optional
	Config map[string]string `json:"config,omitempty"`

	// +optional
	Password string `json:"password,omitempty"`

	// +kubebuilder:default=3306
	// +optional
	Port int32 `json:"port,omitempty"`

	// +optional
	Users []User `json:"users,omitempty"`

	// +optional
	Service ServiceType `json:"service,omitempty"`

	// Upgrade strategy
	// +kubebuilder:default="rollingUpgrade"
	// +kubebuilder:validation:Enum="rollingUpgrade";"all"
	UpgradeStrategy UpgradeStrategyType `json:"upgradeStrategy,omitempty"`

	// +optional
	Pause *PauseGreatDB `json:"pause,omitempty"`

	// +optional
	Restart *RestartGreatDB `json:"restart,omitempty"`

	// +optional
	Delete *DeleteInstance `json:"delete,omitempty"`

	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// +optional
	PrimaryReadable bool `json:"primaryReadable,omitempty"`

	// +optional
	Backup BackupSpec `json:"backup,omitempty"`

	// +optional
	Scaling *Scaling `json:"scaling,omitempty"`

	FailOver FailOver `json:"failOver,omitempty"`

	// +optional
	CloneSource *CloneSource `json:"cloneSource,omitempty"`

	// +optional
	Dashboard GreatDBDashboard `json:"dashboard,omitempty"`

	// +optional
	LogCollection LogCollection `json:"logCollection,omitempty"`
}

// GreatDBPaxosConditions  service state of GreatDBPaxos.
type GreatDBPaxosConditions struct {

	// Type is the type of the condition.
	// +optional
	Type GreatDBPaxosConditionType `json:"type"`

	// Status is the status of the condition.
	// Can be True, False, Unknown.
	// +optional
	Status ConditionStatus `json:"status"`

	// Last time we update the condition.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// GreatDBCondition  instance service state of GreatDB.
type MemberCondition struct {
	// Instance name of greatdb
	// +optional
	Name string `json:"name"`

	// Type is the type of the condition.
	// +optional
	Type MemberConditionType `json:"type"`

	// Last time we update the condition.
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`

	// The role of this instance
	// +optional
	Role MemberRoleType `json:"role"`

	// The name of the PVC used in this instance
	// +optional
	PvcName string `json:"pvcName,omitempty"`

	// Reason for instance creation, optional initialization, expansion, and failover
	// +optional
	CreateType MemberCreateType `json:"createType,omitempty"`

	Index int `json:"index"`

	// Whether you have been added to a cluster
	JoinCluster bool `json:"joinCluster,omitempty"`

	// Instance Access Address
	Address string `json:"address"`

	// instance version
	Version string `json:"version,omitempty"`
}

type UpgradeMember struct {

	// Instance in upgrade
	Upgrading map[string]string `json:"upgrading,omitempty"`

	// Upgrade completed instances
	Upgraded map[string]string `json:"upgraded,omitempty"`

	// Target version for upgrade
	Version string `json:"version,omitempty"`

	// Current version of the cluster
	CurrentVersion string `json:"currentVersion,omitempty"`
}

type RestartMember struct {

	// Restarting an instance
	Restarting map[string]string `json:"restarting,omitempty"`

	// Reboot completed instance
	Restarted map[string]string `json:"restarted,omitempty"`
}

type DashboardStatus struct {
	// Indicates whether the cluster is ready
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Last time the condition transitioned from one status to another.
	// +optional
	LastSyncTime metav1.Time `json:"lastSyncTime,omitempty"`

	// Human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// GreatDBPaxosStatus defines the observed state of GreatDBPaxos
type GreatDBPaxosStatus struct {

	// The state of the cluster
	// +optional
	Status ClusterStatusType `json:"status,omitempty"`
	// The diag state of the cluster
	// +optional
	DiagStatus ClusterDiagStatusType `json:"diagStatus,omitempty"`

	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`

	// The phase of a cluster is a simple, high-level summary of where the cluster is in its lifecycle.
	// +optional
	Phase GreatDBPaxosConditionType `json:"phase,omitempty"`

	// version of cluster
	Version string `json:"version"`

	// Specify the port on which the service runs
	// +optional
	Port int32 `json:"port,omitempty"`

	// Cluster boot was performed through that instance
	// +optional
	BootIns string `json:"bootIns"`

	// Current service state of GreatDBPaxos.
	// +optional
	Conditions []GreatDBPaxosConditions `json:"conditions,omitempty"`

	// Number of GreatDB component desired instances
	// +optional
	Instances int32 `json:"instances"`

	// The actual number of target instances of greatdb
	// +optional
	TargetInstances int32 `json:"targetInstances"`

	// Number of instances with GreatDB component ready
	// +optional
	ReadyInstances int32 `json:"readyInstances"`

	// Currently created instance
	// +optional
	CurrentInstances int32 `json:"currentInstances"`

	// Instances that have joined the cluster
	// +optional
	AvailableReplicas int32 `json:"availableReplicas"`

	// Current service state of GreatDB instance.
	// +optional
	Member []MemberCondition `json:"member,omitempty"`

	// Upgrade member instance records
	// +optional
	UpgradeMember UpgradeMember `json:"upgradeMember,omitempty"`

	// Restarting member instance records
	// +optional
	RestartMember RestartMember `json:"restartMember,omitempty"`

	// Members currently undergoing resizing
	ScaleInMember string `json:"scaleInMember,omitempty"`

	// User initialization result record
	// +optional
	Users []User `json:"users,omitempty"`

	CloneSource CloneSource `json:"cloneSource,omitempty"`

	Dashboard DashboardStatus `json:"dashboard,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=gdb
// +kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="INSTANCE",type="integer",JSONPath=".status.instances"
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyInstances"
// +kubebuilder:printcolumn:name="PORT",type="integer",JSONPath=".status.port"
// +kubebuilder:printcolumn:name="VERSION",type="string",JSONPath=".status.version"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// GreatDBPaxos 主资源定义
type GreatDBPaxos struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GreatDBPaxosSpec   `json:"spec,omitempty"`
	Status GreatDBPaxosStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type GreatDBPaxosList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GreatDBPaxos `json:"items"`
}

func (g GreatDBPaxos) GetClusterDomain() string {

	if g.Spec.ClusterDomain == "" {
		return "cluster.local"
	}

	return g.Spec.ClusterDomain
}

func init() {
	SchemeBuilder.Register(&GreatDBPaxos{}, &GreatDBPaxosList{})
}

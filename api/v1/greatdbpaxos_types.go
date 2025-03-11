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
	v1 "k8s.io/api/core/v1"
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
	// type of service
	// +kubebuilder:default="ClusterIP"
	// +kubebuilder:validation:Enum="ClusterIP";"NodePort";"LoadBalancer"
	Type v1.ServiceType `json:"type"`

	// When the service type is nodePort, the configured node port
	// +optional
	ReadPort int32 `json:"readPort,omitempty"`

	// When the service type is nodePort, the configured node port
	// +optional
	WritePort int32 `json:"writePort,omitempty"`
}

type PauseGreatDB struct {
	// +optional
	Enable bool `json:"enable,omitempty"`
	// pause mode
	// +kubebuilder:default="ins"
	// +kubebuilder:validation:Enum="cluster";"ins"
	Mode PauseModeType `json:"mode,omitempty"`

	// The list of instances that need to be paused only takes effect when mode=ins
	// +optional
	Instances []string `json:"instances,omitempty"`
}

type RestartGreatDB struct {
	// +optional
	Enable bool `json:"enable,omitempty"`
	// restart mode
	// +kubebuilder:default="ins"
	// +kubebuilder:validation:Enum="cluster";"ins"
	Mode RestartModeType `json:"mode,omitempty"`

	// The list of instances that need to be restat only takes effect when mode=ins
	// +optional
	Instances []string `json:"instances,omitempty"`

	// Restart strategy
	// +kubebuilder:default="rolling"
	// +kubebuilder:validation:Enum="rolling";"all"
	Strategy RestartStrategyType `json:"strategy,omitempty"`
}

type DeleteInstance struct {
	// Configure the list of instances that need to be deleted,If the instance does not exist, it will be skipped
	// +optional
	Instances []string `json:"instances,omitempty"`

	// Do you want to synchronize the cleaning of the corresponding PVC
	// +optional
	CleanPvc bool `json:"cleanPvc,omitempty"`
}

type BackupSpec struct {
	// Is the backup service enabled? If it is not enabled, even if a backup plan is created, it will not be backed up
	// +optional
	// +kubebuilder:default=true
	Enable *bool `json:"enable,omitempty"`

	// Compute Resources required by this container. Cannot be updated
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// configuring nfs addresses for backup storage
	// +optional
	NFS *v1.NFSVolumeSource `json:"nfs,omitempty"`
}

type Scaling struct {

	// Shrinkage configuration
	// +optional
	ScaleIn ScaleIn `json:"scaleIn,omitempty"`
	// Configure expansion
	// +optional
	ScaleOut ScaleOut `json:"scaleOut,omitempty"`
}

type ScaleIn struct {
	// Shrinkage strategy,Default fault
	//index: Reduce in reverse order based on index
	//fault: Prioritize scaling down faulty nodes and then reduce them in reverse order based on index
	//define: Shrink the instance specified in the instance field
	// +kubebuilder:default="fault"
	// +kubebuilder:validation:Enum="index";"fault";"define"
	Strategy ScaleInStrategyType `json:"strategy,omitempty"`
	// Effective only when the policy is defined
	Instance []string `json:"instance,omitempty"`
}

type ScaleOut struct {
	// Data source during expansion, optional backup, clone， The default type is backup. If there is no backup, use clone
	// Backup: based on backup data
	// Clone: based on the clone features provided by the database
	// +kubebuilder:default="backup"
	// +kubebuilder:validation:Enum="backup";"clone"
	// +optional
	Source ScaleOutSourceType `json:"source,omitempty"`
}

type FailOver struct {

	// Enable failover function, default: true
	// +kubebuilder:default=true
	Enable *bool `json:"enable,omitempty"`

	// Maximum number of recoverable instances
	// +kubebuilder:default=3
	// +optional
	MaxInstance int32 `json:"maxInstance,omitempty"`

	// The period of failover after the greatdb instance fails
	// +kubebuilder:default="10m"
	// +optional
	Period string `json:"period,omitempty"`

	// An eviction is allowed if at most "maxUnavailable" pods selected by
	// "selector" are unavailable after the eviction, i.e. even in absence of
	// the evicted pod. For example, one can prevent all voluntary evictions
	// by specifying 0. This is a mutually exclusive setting with "minAvailable".
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// Whether to automatically shrink the expansion node after the recovery of the faulty node, default： false
	// +kubebuilder:default=false
	AutoScaleIn *bool `json:"autoScaleIn,omitempty"`
}

type CloneSource struct {
	// The namespace where the clone source is located, The namespace where the clone source is located, defaults to the command space set by the cluster
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// To clone the source (data source),
	// it is necessary to ensure that the cluster has a successful backup record and that the storage configuration remains consistent
	ClusterName string `json:"clusterName,omitempty"`

	// Backup records, priority higher than ClusterName
	// +optional
	BackupRecordName string `json:"backupRecordName,omitempty"`
}

type GreatDBDashboard struct {
	// Eabled Dashboard components
	// +optional
	Enable bool `json:"enable"`

	// image used by the component
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// Compute Resources required by this container. Cannot be updated
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// volumeClaimTemplates is a list of claims that pods are allowed to reference.
	// +optional
	PersistentVolumeClaimSpec v1.PersistentVolumeClaimSpec `json:"persistentVolumeClaimSpec,omitempty"`

	Config map[string]string `json:"config,omitempty"`

	// Annotations for the component.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels for the component.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// LogCollection is the desired state of Promtail sidecar
type LogCollection struct {
	// image used by the component
	Image string `json:"image"`

	LokiClient string `json:"lokiClient,omitempty"`
	// Compute Resources required by this container. Cannot be updated
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// GreatDBPaxosSpec defines the desired state of GreatDBPaxos
type GreatDBPaxosSpec struct {
	// set cluster affinity
	// +optional
	Affinity *v1.Affinity `json:"affinity,omitempty"`

	// PvReclaimPolicy Corresponding to pv.spec.persistentVolumeReclaimPolicy
	// +kubebuilder:default="Retain"
	// +kubebuilder:validation:Enum="Delete";"Retain"
	PvReclaimPolicy v1.PersistentVolumeReclaimPolicy `json:"pvReclaimPolicy,omitempty"`

	//  Corresponding to pv.spec.securityContext
	// +optional
	PodSecurityContext v1.PodSecurityContext `json:"podSecurityContext,omitempty"`

	// SecretName is used to configure cluster account password authentication
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName,omitempty"`

	// Set the binding priority name for cluster scheduling
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`

	//  Image pull policy. One of Always, Never, IfNotPresent. Defaults to Always
	// if :latest tag is specified, or IfNotPresent otherwise. Cannot be updated.
	// +optional
	// +kubebuilder:default="Always"
	// +kubebuilder:validation:Enum="Always";"IfNotPresent"
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images.
	// +optional
	ImagePullSecrets []v1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Start manual maintenance mode for the cluster
	// +optional
	MaintenanceMode bool `json:"maintenanceMode"`

	// The cluster domain of the current k8s cluster
	// +kubebuilder:default="cluster.local"
	// +optional
	ClusterDomain string `json:"clusterDomain,omitempty"`

	// number of instances of the GreatDBPaxos, default:3
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=2
	// +optional
	Instances int32 `json:"instances,omitempty"`

	// image used by the GreatDBPaxos
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// version of cluster
	Version string `json:"version"`

	// Compute Resources required by this container. Cannot be updated
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// VolumeClaimTemplates is a list of claims that pods are allowed to reference.
	// +optional
	VolumeClaimTemplates v1.PersistentVolumeClaimSpec `json:"volumeClaimTemplates,omitempty"`

	// Customized configuration parameters for database components
	// +optional
	Config map[string]string `json:"config,omitempty"`

	// Password Cluster root user password
	// +optional
	Password string `json:"password,omitempty"`

	// Specify the port on which the service runs
	// +kubebuilder:default=3306
	// +optional
	Port int32 `json:"port,omitempty"`

	// Initialize user list
	// +optional
	Users []User `json:"users,omitempty"`

	// Configure the type of service
	// +optional
	Service ServiceType `json:"service,omitempty"`

	// Upgrade strategy
	// +kubebuilder:default="rollingUpgrade"
	// +kubebuilder:validation:Enum="rollingUpgrade";"all"
	UpgradeStrategy UpgradeStrategyType `json:"upgradeStrategy,omitempty"`

	// Configure GreatDB restart
	// +optional
	Pause *PauseGreatDB `json:"pause,omitempty"`

	// Configure GreatDB restart
	// +optional
	Restart *RestartGreatDB `json:"restart,omitempty"`

	// Configure GreatDB restart
	Delete *DeleteInstance `json:"delete,omitempty"`

	// Annotations for the component.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels for the component.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Set whether the main node is readable
	// +optional
	PrimaryReadable bool `json:"primaryReadable,omitempty"`

	// configure backup support
	// +optional
	Backup BackupSpec `json:"backup,omitempty"`

	// Expansion and contraction configuration
	// +optional
	Scaling *Scaling `json:"scaling,omitempty"`

	// Failover Configuration
	FailOver FailOver `json:"failOver,omitempty"`

	// Configure the clone source and create a new cluster based on existing data
	// +optional
	CloneSource *CloneSource `json:"cloneSource,omitempty"`

	// GreatDBDashboar Is the configuration of this Dashboard component
	// +optional
	Dashboard GreatDBDashboard `json:"dashboard,omitempty"`

	// LogCollection Is the configuration of this promtail component
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
// +kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="INSTANCE",type="string",JSONPath=".status.instances"
// +kubebuilder:printcolumn:name="READYINSTANCE",type="string",JSONPath=".status.readyInstances"
// +kubebuilder:printcolumn:name="PORT",type="string",JSONPath=".status.port"
// +kubebuilder:printcolumn:name="VERSION",type="string",JSONPath=".status.version"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:categories=all,path=greatdbpaxoses,scope=Namespaced,shortName=gdb,singular=greatdbpaxos

// GreatDBPaxos is the Schema for the greatdbpaxos API
type GreatDBPaxos struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GreatDBPaxosSpec   `json:"spec,omitempty"`
	Status GreatDBPaxosStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GreatDBPaxosList contains a list of GreatDBPaxos
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

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GreatDBClusterSpec defines the desired state of GreatDBCluster
type GreatDBBackupScheduleSpec struct {

	// ClusterName is cluster name
	// Backup will only occur when the cluster is available
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName,omitempty"`

	// Backup from a specified instance. By default, select the primary SECONDARY node. If there is no SECONDARY node, select the primary node for backup
	// +optional
	InstanceName string `json:"instanceName,omitempty"`

	//  This flag tells the controller to suspend subsequent executions, it does
	// not apply to already started executions. Defaults to false.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// Scheduler is list of backup scheduler.
	// +optional
	Schedulers []BackupScheduler `json:"schedulers,omitempty"`
}

// GreatDBBackupScheduleStatus defines the observed state of GreatDBBackup
type GreatDBBackupScheduleStatus struct {

	// Human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`

	//  This flag tells the controller to suspend subsequent executions, it does
	// not apply to already started executions. Defaults to false.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// Current application plan
	// +optional
	Schedulers []BackupScheduler `json:"schedulers,omitempty"`
}

type BackupType string

var (
	BackupTypeFull      BackupType = "full"
	BackupTypeIncrement BackupType = "inc"
)

type BackupResourceType string

var (
	GreatDBBackupResourceType BackupResourceType = "greatdb"
)

type BackupScheduler struct {
	// Name used by the component
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Backup type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum="full";"inc"
	BackupType BackupType `json:"backupType"`

	// Backup method
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum="greatdb"
	BackupResource BackupResourceType `json:"backupResource"`

	// The schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
	// If left blank, initiate an immediate backup
	// +optional
	Schedule string `json:"schedule"`

	// Regular cleaning, default not to clean, configuration of this function must ensure that the storage has cleaning permissions,support units[h,d]
	// +optional
	Clean string `json:"clean,omitempty"`

	// select storage spec
	SelectStorage BackupStorageSpec `json:"selectStorage,omitempty"`
}

type BackupStorageSpec struct {
	// The storage type used for backup. When selecting the NFS type, nfs must be specified for backup when creating the cluster, otherwise it cannot be backed up
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum="s3";"nfs"
	Type BackupStorageType `json:"type"`

	// S3 storage
	// +optional
	S3 *BackupStorageS3Spec `json:"s3,omitempty"`

	// Upload to file server
	// +optional
	// UploadServer *BackupStorageUploadServerSpec `json:"UploadServer,omitempty"`
}

type BackupStorageS3Spec struct {
	Bucket      string `json:"bucket"`
	EndpointURL string `json:"endpointUrl"`
	AccessKey   string `json:"accessKey"`
	SecretKey   string `json:"secretKey"`
}

type BackupStorageUploadServerSpec struct {
	Address string `json:"address,omitempty"`
	Port    int    `json:"port,omitempty"`
}

type BackupStorageType string

const (
	BackupStorageUploadServer BackupStorageType = "uploadServer"
	BackupStorageNFS          BackupStorageType = "nfs"
	BackupStorageS3           BackupStorageType = "s3"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=all,path=greatdbbackupschedules,scope=Namespaced,shortName=gbs,singular=greatdbbackupschedule

// GreatDBBackupSchedule is the Schema for the GreatDBBackupSchedule API
type GreatDBBackupSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GreatDBBackupScheduleSpec   `json:"spec,omitempty"`
	Status GreatDBBackupScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// GreatDBBackupScheduleList contains a list of GreatDBBackupSchedule
type GreatDBBackupScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GreatDBBackupSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GreatDBBackupSchedule{}, &GreatDBBackupScheduleList{})
}

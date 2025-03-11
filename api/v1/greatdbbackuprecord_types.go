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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// service state type of GreatDBBackupRecord
type GreatDBBackupRecordConditionType string

const (
	GreatDBBackupRecordConditionInit    GreatDBBackupRecordConditionType = "Init"
	GreatDBBackupRecordConditionRunning GreatDBBackupRecordConditionType = "Running"
	GreatDBBackupRecordConditionSuccess GreatDBBackupRecordConditionType = "Success"
	GreatDBBackupRecordConditionError   GreatDBBackupRecordConditionType = "Error"
	GreatDBBackupRecordConditionUnknown GreatDBBackupRecordConditionType = "Unknown"
)

// GreatDBClusterSpec defines the desired state of GreatDBCluster
type GreatDBBackupRecordSpec struct {

	// ClusterName is cluster name
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName,omitempty"`

	// Backup from a specified instance
	InstanceName string `json:"instanceName,omitempty"`

	// Backup type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum="full";"inc"
	BackupType BackupType `json:"backupType"`

	// Backup method
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum="greatdb"
	BackupResource BackupResourceType `json:"backupResource"`

	// Regular cleaning, default not to clean, configuration of this function must ensure that the storage has cleaning permissions,support units[h,d]
	// +optional
	Clean string `json:"clean,omitempty"`

	// select storage spec
	SelectStorage BackupStorageSpec `json:"selectStorage,omitempty"`
}

// GreatDBBackupRecordStatus defines the observed state of GreatDBBackupRecord
type GreatDBBackupRecordStatus struct {

	// Status is the status of the condition.
	// Can be True, False, Unknown.
	// +optional
	Status GreatDBBackupRecordConditionType `json:"status"`

	// CompletedAt is CompletedAt.
	// +optional
	CompletedAt *metav1.Time `json:"completed,omitempty"`

	// +optional
	Message string `json:"message,omitempty"`

	// +optional
	BackupType string `json:"backupType,omitempty"`

	// +optional
	FromLsn string `json:"fromLsn,omitempty"`

	// +optional
	ToLsn string `json:"toLsn,omitempty"`

	// +optional
	LastLsn string `json:"lastLsn,omitempty"`

	// +optional
	BackupPath []string `json:"backupPath,omitempty"`

	// +optional
	BackupIncFromLsn string `json:"backupIncFromLsn,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="TYPE",type="string",JSONPath=".status.backupType"
// +kubebuilder:printcolumn:name="PATH",type="string",JSONPath=".status.backupPath"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:categories=all,path=greatdbbackuprecords,scope=Namespaced,shortName=gbr,singular=greatdbbackuprecord

// GreatDBBackupRecord is the Schema for the GreatDBBackupRecord API
type GreatDBBackupRecord struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GreatDBBackupRecordSpec   `json:"spec,omitempty"`
	Status GreatDBBackupRecordStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// GreatDBBackupRecordList contains a list of GreatDBBackupRecord
type GreatDBBackupRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GreatDBBackupRecord `json:"items"`
}

func (backupRecord *GreatDBBackupRecord) BackupSvcName() (name string) {
	return backupRecord.Spec.ClusterName + "-greatdb-backup"
}

func (backupRecord *GreatDBBackupRecord) UploadServerAddress(domain string) (name string) {
	return fmt.Sprintf("%s.%s.svc.%s", backupRecord.BackupSvcName(), backupRecord.Namespace, domain)
}

// greatdb pod fqdn
func (backupRecord *GreatDBBackupRecord) BackupServerAddress(domain string) (name string) {
	nameSvc := backupRecord.Spec.InstanceName + "." + backupRecord.Spec.ClusterName + "-greatdb"
	return fmt.Sprintf("%s.%s.svc.%s", nameSvc, backupRecord.Namespace, domain)
}

func init() {
	SchemeBuilder.Register(&GreatDBBackupRecord{}, &GreatDBBackupRecordList{})
}

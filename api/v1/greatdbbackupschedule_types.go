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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GreatDBBackupScheduleSpec defines the desired state of GreatDBBackupSchedule.
type GreatDBBackupScheduleSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of GreatDBBackupSchedule. Edit greatdbbackupschedule_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// GreatDBBackupScheduleStatus defines the observed state of GreatDBBackupSchedule.
type GreatDBBackupScheduleStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// GreatDBBackupSchedule is the Schema for the greatdbbackupschedules API.
type GreatDBBackupSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GreatDBBackupScheduleSpec   `json:"spec,omitempty"`
	Status GreatDBBackupScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GreatDBBackupScheduleList contains a list of GreatDBBackupSchedule.
type GreatDBBackupScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GreatDBBackupSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GreatDBBackupSchedule{}, &GreatDBBackupScheduleList{})
}

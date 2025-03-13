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
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	greatdbv1 "greatdb.com/greatdb-operator/api/v1"
	"greatdb.com/greatdb-operator/internal/utils/tools"
)

// nolint:unused
// log is for logging in this package.
var greatdbbackuprecordlog = logf.Log.WithName("greatdbbackuprecord-resource")

// SetupGreatDBBackupRecordWebhookWithManager registers the webhook for GreatDBBackupRecord in the manager.
func SetupGreatDBBackupRecordWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&greatdbv1.GreatDBBackupRecord{}).
		WithValidator(&GreatDBBackupRecordCustomValidator{Client: mgr.GetClient()}).
		WithDefaulter(&GreatDBBackupRecordCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-greatdb-greatdb-com-v1-greatdbbackuprecord,mutating=true,failurePolicy=fail,sideEffects=None,groups=greatdb.greatdb.com,resources=greatdbbackuprecords,verbs=create;update,versions=v1,name=mgreatdbbackuprecord-v1.kb.io,admissionReviewVersions=v1

// GreatDBBackupRecordCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind GreatDBBackupRecord when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type GreatDBBackupRecordCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &GreatDBBackupRecordCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind GreatDBBackupRecord.
func (d *GreatDBBackupRecordCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	greatdbbackuprecord, ok := obj.(*greatdbv1.GreatDBBackupRecord)

	if !ok {
		return fmt.Errorf("expected an GreatDBBackupRecord object but got %T", obj)
	}
	greatdbbackuprecordlog.Info("Defaulting for GreatDBBackupRecord", "name", greatdbbackuprecord.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-greatdb-greatdb-com-v1-greatdbbackuprecord,mutating=false,failurePolicy=fail,sideEffects=None,groups=greatdb.greatdb.com,resources=greatdbbackuprecords,verbs=create;update,versions=v1,name=vgreatdbbackuprecord-v1.kb.io,admissionReviewVersions=v1

// GreatDBBackupRecordCustomValidator struct is responsible for validating the GreatDBBackupRecord resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
// 新增客户端依赖
type GreatDBBackupRecordCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &GreatDBBackupRecordCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type GreatDBBackupRecord.
func (v *GreatDBBackupRecordCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {

	backupRecord, ok := obj.(*greatdbv1.GreatDBBackupRecord)
	if !ok {
		return nil, fmt.Errorf("expected a GreatDBBackupRecord object but got %T", obj)
	}

	greatdbbackuprecordlog.Info("Validation for GreatDBBackupRecord upon creation", "name", backupRecord.GetName())

	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec")

	if err := ValidateClusterName(ctx, v.Client, fieldPath, backupRecord.Namespace, backupRecord.Spec.ClusterName); err != nil {
		allErrs = append(allErrs, err)
	}

	allErrs = append(allErrs, ValidateBackupRecord(ctx, v.Client, fieldPath, backupRecord)...)

	if err := ValidateInstanceName(ctx, v.Client, fieldPath.Child("instanceName"), backupRecord.Namespace, backupRecord.Spec.ClusterName, backupRecord.Spec.InstanceName); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) > 0 {
		return nil, fmt.Errorf("validation failed: %v", allErrs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type GreatDBBackupRecord.
func (v *GreatDBBackupRecordCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	greatdbbackuprecord, ok := newObj.(*greatdbv1.GreatDBBackupRecord)
	if !ok {
		return nil, fmt.Errorf("expected a GreatDBBackupRecord object for the newObj but got %T", newObj)
	}
	greatdbbackuprecordlog.Info("Validation for GreatDBBackupRecord upon update", "name", greatdbbackuprecord.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type GreatDBBackupRecord.
func (v *GreatDBBackupRecordCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	greatdbbackuprecord, ok := obj.(*greatdbv1.GreatDBBackupRecord)
	if !ok {
		return nil, fmt.Errorf("expected a GreatDBBackupRecord object but got %T", obj)
	}
	greatdbbackuprecordlog.Info("Validation for GreatDBBackupRecord upon deletion", "name", greatdbbackuprecord.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}

func ValidateBackupRecord(ctx context.Context, v client.Client, fieldPath *field.Path, backupRecord *greatdbv1.GreatDBBackupRecord) field.ErrorList {
	var allErrs field.ErrorList
	// 校验集群是否存在
	cluster := &greatdbv1.GreatDBPaxos{}
	if err := v.Get(ctx, types.NamespacedName{Namespace: backupRecord.Namespace, Name: backupRecord.Spec.ClusterName}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}

		allErrs = append(allErrs, field.InternalError(fieldPath, err))
	}

	// 校验Clean字段格式
	if backupRecord.Spec.Clean != "" {
		if _, err := tools.StringToDuration(backupRecord.Spec.Clean); err != nil {
			allErrs = append(allErrs, field.Invalid(
				fieldPath.Child("clean"),
				backupRecord.Spec.Clean,
				fmt.Sprintf("Invalid clean format: %v", err),
			))
		}
	}

	// 校验存储配置
	switch backupRecord.Spec.SelectStorage.Type {
	case greatdbv1.BackupStorageNFS:
		if cluster.Spec.Backup.NFS == nil {
			allErrs = append(allErrs, field.Required(
				fieldPath.Child("selectStorage", "type"),
				"Cluster not configured with NFS backup",
			))
		}
	case greatdbv1.BackupStorageS3:
		s3 := backupRecord.Spec.SelectStorage.S3
		if s3 == nil || s3.Bucket == "" || s3.EndpointURL == "" || s3.AccessKey == "" || s3.SecretKey == "" {
			allErrs = append(allErrs, field.Required(
				fieldPath.Child("selectStorage", "s3"),
				"S3 configuration cannot be empty",
			))
		}
	}

	return allErrs
}

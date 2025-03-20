package secret

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	greatdbv1 "greatdb.com/greatdb-operator/api/v1"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager"
	dblog "greatdb.com/greatdb-operator/internal/utils/log"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

const (
	EventMessageCreateClusterSecret          = "The system creates a default secret, but this is not recommended. It is recommended to create a secret yourself"
	EventMessageCreateClusterSecretSucceeded = "Cluster default secret created successfully"
	EventReasonCreateClusterSecret           = "create cluster secret"
	EventMessageUpdateClusterSecret          = "A complete secret should be provided"
)

type Manager struct {
	Client   client.Client
	Recorder record.EventRecorder
}

func (r *Manager) Sync(ctx context.Context, cluster *greatdbv1.GreatDBPaxos) (err error) {
	ns, secretName := cluster.Namespace, cluster.Spec.SecretName
	// If the user has set the secretName, you need to check whether the secret really exists. If it does not exist, the operator should create it
	secret, err := r.GetSecret(ctx, ns, secretName)

	if err != nil {
		return err
	}

	// Already exists, update directly
	if secret != nil {
		if err = r.UpdateSecret(ctx, cluster, secret); err != nil {
			return err
		}
		cluster.Spec.SecretName = secret.GetName()
		return nil
	}

	if err = r.CreateSecret(ctx, cluster); err != nil {
		return err
	}
	if err = r.UpdateClusterSecretName(ctx, cluster, secretName); err != nil {
		return err
	}

	return nil
}

func (r *Manager) GetSecret(ctx context.Context, ns, name string) (*corev1.Secret, error) {

	if name == "" {
		return nil, nil
	}

	var secret = &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, secret)
	if err == nil {
		return secret, nil
	}

	if k8serrors.IsNotFound(err) {
		dblog.Log.Infof("This secret %s/%s does not exist", ns, name)
		return nil, nil
	}

	dblog.Log.Errorf("Failed to  get secret %s/%s, message: %s", ns, name, err.Error())
	return nil, err
}

// CreateSecret Create a secret for the cluster
func (r *Manager) CreateSecret(ctx context.Context, cluster *greatdbv1.GreatDBPaxos) error {
	// The cluster starts to clean, and no more resources need to be created
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}

	// recorder event
	r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCreateClusterSecret, EventMessageCreateClusterSecret)

	//  a new secret
	secret, err := r.NewClusterSecret(cluster)
	if err != nil {
		dblog.Log.Errorf("failed to new secret %s", err.Error())
		return err
	}

	cluster.Spec.SecretName = secret.GetName()

	// create secret
	err = r.Client.Create(ctx, secret)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			cluster.Spec.SecretName = secret.GetName()
			dblog.Log.Info(err.Error())
			return nil
		}
		dblog.Log.Errorf("failed to create secret %s/%s , messages: %s", secret.Namespace, secret.Name, err.Error())
		return err
	}

	r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventMessageCreateClusterSecretSucceeded, EventMessageCreateClusterSecret)

	return nil
}

// NewClusterSecret Returns the available cluster secret
func (r *Manager) NewClusterSecret(cluster *greatdbv1.GreatDBPaxos) (secret *corev1.Secret, err error) {
	clusterName, namespace := cluster.Name, cluster.Namespace
	name := r.getSecretName(clusterName)
	labels := r.GetLabels(clusterName)

	data := r.getDefaultSecretData(cluster)
	owner := resourcemanager.GetGreatDBClusterOwnerReferences(clusterName, cluster.UID)

	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			Finalizers:      []string{resourcemanager.FinalizersGreatDBCluster},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Data: data,
	}

	return secret, nil
}

// getSecretName Returns the  secret names
func (r *Manager) getSecretName(clusterName string) string {
	return fmt.Sprintf("%s-%s", clusterName, "secret")
}

// GetLabels  Return to the default label settings
func (r *Manager) GetLabels(name string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resourcemanager.AppKubeNameLabelKey] = resourcemanager.AppKubeNameLabelValue
	labels[resourcemanager.AppkubeManagedByLabelKey] = resourcemanager.AppkubeManagedByLabelValue
	labels[resourcemanager.AppKubeInstanceLabelKey] = name
	return

}

// getDefaultSecretData Return to the default secret data
func (r *Manager) getDefaultSecretData(cluster *greatdbv1.GreatDBPaxos) (data map[string][]byte) {
	data = make(map[string][]byte)
	rootPwd := resourcemanager.RootPasswordDefaultValue
	data[resourcemanager.RootPasswordKey] = []byte(rootPwd)
	user, pwd := resourcemanager.GetClusterUser(cluster)
	data[resourcemanager.ClusterUserKey] = []byte(user)
	data[resourcemanager.ClusterUserPasswordKey] = []byte(pwd)
	return
}

// UpdateSecret  Update existing secret
func (r *Manager) UpdateSecret(ctx context.Context, cluster *greatdbv1.GreatDBPaxos, secret *corev1.Secret) error {

	// clean cluster
	if !cluster.DeletionTimestamp.IsZero() {
		if len(secret.Finalizers) == 0 {
			return nil
		}

		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
		err := r.Client.Patch(ctx, secret, client.RawPatch(types.JSONPatchType, []byte(patch)))
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of secret %s/%s,message: %s", secret.Namespace, secret.Name, err.Error())
		}

		return nil
	}

	needUpdate := r.SetDefaultValue(cluster, secret)
	if !needUpdate {
		dblog.Log.V(3).Infof("Secret %s/%s does not need to be updated", secret.Namespace, secret.Name)
		return nil
	}

	err := r.Client.Update(context.TODO(), secret)
	if err != nil {
		dblog.Log.Errorf("failed to update secret %s/%s, message: %s", secret.Namespace, secret.Name, err.Error())
		return err
	}

	r.Recorder.Eventf(secret, corev1.EventTypeWarning, "update secret", EventMessageUpdateClusterSecret)

	return nil
}

// SetDefaultValue Set secret default value
func (r *Manager) SetDefaultValue(cluster *greatdbv1.GreatDBPaxos, secret *corev1.Secret) bool {
	needUpdate := false

	if r.SetDefaultData(secret, cluster) {
		needUpdate = true
	}

	if r.updateMeta(cluster, secret) {
		needUpdate = true
	}

	return needUpdate
}

// SetDefaultData  Set missing required fields
func (r *Manager) SetDefaultData(secret *corev1.Secret, cluster *greatdbv1.GreatDBPaxos) bool {
	needUpdate := false

	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}

	if _, ok := secret.Data[resourcemanager.ClusterUserKey]; ok {
		delete(secret.Data, resourcemanager.ClusterUserKey)
		needUpdate = true
	}

	if _, ok := secret.Data[resourcemanager.ClusterUserPasswordKey]; ok {
		delete(secret.Data, resourcemanager.ClusterUserPasswordKey)
		needUpdate = true
	}

	data := r.getDefaultSecretData(cluster)

	for key, value := range data {
		if _, ok := secret.Data[key]; !ok {
			secret.Data[key] = []byte(value)
			needUpdate = true
		}

	}

	return needUpdate
}

func (r *Manager) updateMeta(cluster *greatdbv1.GreatDBPaxos, secret *corev1.Secret) bool {
	needUpdate := false

	// update labels
	if r.UpdateLabel(cluster.Name, secret) {
		needUpdate = true
	}

	// update ownerReferences
	if secret.OwnerReferences == nil {
		secret.OwnerReferences = make([]metav1.OwnerReference, 0, 1)
	}
	exist := false
	for _, own := range secret.OwnerReferences {
		if own.UID == cluster.UID {
			exist = true
			break
		}
	}
	if !exist {

		owner := resourcemanager.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
		// No need to consider references from other owners
		secret.OwnerReferences = []metav1.OwnerReference{owner}
		needUpdate = true
	}

	return needUpdate

}

func (r *Manager) UpdateLabel(clusterName string, secret *corev1.Secret) bool {
	needUpdate := false
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}

	labels := r.GetLabels(clusterName)
	for key, value := range labels {
		if v, ok := secret.Labels[key]; !ok || v != value {
			needUpdate = true
			secret.Labels[key] = value
		}
	}

	return needUpdate

}

// UpdateClusterSecretName 更新 GreatDBPaxos 资源的 spec.secretName 字段
func (r *Manager) UpdateClusterSecretName(ctx context.Context, cluster *greatdbv1.GreatDBPaxos, secretName string) error {
	// 1. 构造 JSON Patch
	patch := client.RawPatch(types.JSONPatchType, []byte(fmt.Sprintf(
		`[{"op":"replace","path":"/spec/secretName","value":"%s"}]`,
		secretName,
	)))

	// 2. 应用 Patch
	if err := r.Client.Patch(ctx, cluster, patch); err != nil {
		// 使用 Kubebuilder 内置日志记录
		dblog.Log.Errorf("failed to update field secretName of  cluster %s/%s, message: %s", cluster.Namespace, cluster.Name, err.Error())

		return fmt.Errorf("failed to patch secretName: %v", err)
	}

	return nil
}

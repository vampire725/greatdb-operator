package greatdbpaxos

import (
	"context"
	"encoding/json"
	"fmt"

	v1alpha1 "greatdb.com/greatdb-operator/api/v1"
	resources "greatdb.com/greatdb-operator/internal/controller/resourcemanager"
	dblog "greatdb.com/greatdb-operator/internal/utils/log"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	// dblog "greatdb-operator/pkg/utils/log"
)

func (great GreatDBManager) SyncPvc(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) error {

	if member.PvcName == "" {
		member.PvcName = member.Name
	}

	pvc, err := great.Lister.PvcLister.PersistentVolumeClaims(cluster.Namespace).Get(member.PvcName)
	if err != nil {

		if k8serrors.IsNotFound(err) {
			return great.CreatePvc(cluster, member)
		}
		dblog.Log.Reason(err).Errorf("failed to lister pvc %s/%s", cluster.Namespace, member.PvcName)

	}
	newPvc := pvc.DeepCopy()

	err = great.UpdatePvc(cluster, newPvc)
	if err != nil {
		return err
	}

	err = great.updatePv(newPvc, cluster.Spec.PvReclaimPolicy)
	if err != nil {
		return err
	}

	return nil

}

func (great GreatDBManager) CreatePvc(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) error {

	if member.PvcName == "" {
		member.PvcName = member.Name
	}

	pvc := great.NewPvc(cluster, member)

	_, err := great.Client.KubeClientset.CoreV1().PersistentVolumeClaims(cluster.Namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			//  need to add a label to the configmap to ensure that the operator can monitor it
			labels, _ := json.Marshal(pvc.Labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labels)
			_, err = great.Client.KubeClientset.CoreV1().PersistentVolumeClaims(cluster.Namespace).Patch(
				context.TODO(), pvc.Name, types.StrategicMergePatchType, []byte(data), metav1.PatchOptions{})
			if err != nil {
				dblog.Log.Errorf("failed to update the labels of pod, message: %s", err.Error())
				return err
			}
			return nil
		}
		dblog.Log.Reason(err).Error("failed to create pvc")
		return err
	}
	dblog.Log.Infof("successfully created PVC %s/%s", pvc.Namespace, pvc.Name)

	return nil

}

// GetPvcLabels  Return to the default label settings
func (great GreatDBManager) GetPvcLabels(name, podName string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeInstanceLabelKey] = name
	labels[resources.AppKubePodLabelKey] = podName
	return

}

func (great GreatDBManager) NewPvc(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (pvc *corev1.PersistentVolumeClaim) {

	owner := resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            member.PvcName,
			Namespace:       cluster.Namespace,
			Labels:          great.GetPvcLabels(cluster.Name, member.Name),
			Finalizers:      []string{resources.FinalizersGreatDBCluster},
			OwnerReferences: []metav1.OwnerReference{owner},
			Annotations:     cluster.Spec.Annotations,
		},
		Spec: cluster.Spec.VolumeClaimTemplates,
	}
	return

}

func (great GreatDBManager) UpdatePvc(cluster *v1alpha1.GreatDBPaxos, pvc *corev1.PersistentVolumeClaim) error {

	if !cluster.DeletionTimestamp.IsZero() {
		if len(pvc.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
		_, err := great.Client.KubeClientset.CoreV1().PersistentVolumeClaims(pvc.Namespace).Patch(
			context.TODO(), pvc.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of pvc %s/%s,message: %s", pvc.Namespace, pvc.Name, err.Error())
			return err
		}

		return nil

	}

	var changeArr = make([]string, 0)
	var exist bool

	// update Finalizers
	if pvc.ObjectMeta.Finalizers == nil {
		pvc.ObjectMeta.Finalizers = make([]string, 0, 1)
	}

	for _, value := range pvc.ObjectMeta.Finalizers {
		if value == resources.FinalizersGreatDBCluster {
			exist = true
			break
		}
	}
	if !exist {
		pvc.ObjectMeta.Finalizers = append(pvc.ObjectMeta.Finalizers, resources.FinalizersGreatDBCluster)
		fin, _ := json.Marshal(pvc.ObjectMeta.Finalizers)
		data := fmt.Sprintf(`{"op":"add","path":"/metadata/finalizers","value":%s}`, fin)
		changeArr = append(changeArr, data)
	}

	// update OwnerReferences
	if pvc.ObjectMeta.OwnerReferences == nil {
		pvc.ObjectMeta.OwnerReferences = make([]metav1.OwnerReference, 0, 1)
	}
	exist = false
	for _, value := range pvc.ObjectMeta.OwnerReferences {
		if value.UID == cluster.UID {
			exist = true
			break
		}
	}

	if !exist {
		owner := resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)

		ownerRef, _ := json.Marshal(owner)
		data := fmt.Sprintf(`{"op":"add","path":"/metadata/ownerReferences","value":[%s]}`, ownerRef)
		changeArr = append(changeArr, data)
	}

	result := pvc.Spec.Resources.Requests.Storage().Cmp(*cluster.Spec.VolumeClaimTemplates.Resources.Requests.Storage())

	// old < new
	if result == -1 {
		allow, reason, err := great.AllowVolumeExpansion(pvc)

		if err != nil {
			dblog.Log.Reason(err).Error(reason)
			return err
		}
		if allow {
			storage := make(corev1.ResourceList)
			storage[corev1.ResourceStorage] = *cluster.Spec.VolumeClaimTemplates.Resources.Requests.Storage()

			sto, _ := json.Marshal(storage)
			data := fmt.Sprintf(`{"op":"replace","path":"/spec/resources/requests","value":%s}`, sto)
			changeArr = append(changeArr, data)

		} else {
			message := fmt.Sprintf("%s to %s:  %s", pvc.Spec.Resources.Requests.Storage().String(), cluster.Spec.VolumeClaimTemplates.Resources.Requests.Storage().String(), reason)
			great.Recorder.Event(cluster, corev1.EventTypeWarning, StorageVerticalExpansionNotSupport, message)
			cluster.Spec.VolumeClaimTemplates.Resources.Requests[corev1.ResourceStorage] = pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		}

	} else if result == 1 {
		message := fmt.Sprintf("%s to %s", pvc.Spec.Resources.Requests.Storage().String(), cluster.Spec.VolumeClaimTemplates.Resources.Requests.Storage().String())
		great.Recorder.Event(cluster, corev1.EventTypeWarning, StorageVerticalShrinkagegprohibit, message)
		cluster.Spec.VolumeClaimTemplates.Resources.Requests[corev1.ResourceStorage] = pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	}

	patch := ""
	for _, value := range changeArr {
		if patch == "" {
			patch += "[" + value
		} else {
			patch += "," + value
		}
	}
	patch += "]"

	if len(changeArr) > 0 {
		_, err := great.Client.KubeClientset.CoreV1().PersistentVolumeClaims(pvc.Namespace).Patch(
			context.TODO(), pvc.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to update pvc %s/%s,message: %s", pvc.Namespace, pvc.Name, err.Error())
			return err
		}
	}

	return nil

}

func (great GreatDBManager) updatePv(pvc *corev1.PersistentVolumeClaim, reclaimPolicy corev1.PersistentVolumeReclaimPolicy) error {

	if pvc.Spec.VolumeName == "" {
		return nil
	}

	pv, err := great.GetPv(pvc.Spec.VolumeName)
	if err != nil {
		return err
	}
	if pv.Spec.PersistentVolumeReclaimPolicy == reclaimPolicy {
		return nil
	}

	patch := fmt.Sprintf(`[{"op":"replace","path":"/spec/persistentVolumeReclaimPolicy","value":"%s"}]`, reclaimPolicy)

	_, err = great.Client.KubeClientset.CoreV1().PersistentVolumes().Patch(context.TODO(),
		pv.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		dblog.Log.Errorf(err.Error())
		return err
	}
	dblog.Log.Infof("Successfully synchronized pv %s/%s", pvc.Namespace, pvc.Name)
	return err

}

func (great GreatDBManager) GetPv(pvName string) (pv *corev1.PersistentVolume, err error) {

	pv, err = great.Lister.PvLister.Get(pvName)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			dblog.Log.Errorf("failed to lister pv %s", pvName)
			return
		}
		pv, err = great.Client.KubeClientset.CoreV1().PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to get pv %s", pvName)
			return nil, err
		}

		if pv.ObjectMeta.Labels == nil {
			pv.ObjectMeta.Labels = make(map[string]string)
		}

		pv.ObjectMeta.Labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
		pv.ObjectMeta.Labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
		labels, _ := json.Marshal(pv.ObjectMeta.Labels)
		data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labels)
		_, err = great.Client.KubeClientset.CoreV1().PersistentVolumes().Patch(context.TODO(), pvName, types.StrategicMergePatchType, []byte(data), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Reason(err).Errorf("failed to update the label of PV %s", pvName)
		}
	}

	return
}

func (great GreatDBManager) AllowVolumeExpansion(pvc *corev1.PersistentVolumeClaim) (allow bool, reason string, err error) {

	if pvc.Spec.StorageClassName == nil {
		reason = fmt.Sprintf("PVC %s/%s is not bound to a storage class, and the cluster environment has not set a default storage class", pvc.Namespace, pvc.Name)
		return allow, reason, nil
	}
	storageClassName := *pvc.Spec.StorageClassName
	if storageClassName == "" {
		reason = fmt.Sprintf("PVC %s/%s is not bound to a storage class, and the cluster environment has not set a default storage class", pvc.Namespace, pvc.Name)
		return allow, reason, nil
	}

	storageClass, err := great.Client.KubeClientset.StorageV1().StorageClasses().Get(context.TODO(), storageClassName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			reason = fmt.Sprintf("storageClass %s not exist", storageClassName)
			dblog.Log.Errorf(reason)
			return allow, reason, nil
		}
		dblog.Log.Errorf("failed to get StorageClass %s, message: %s", storageClassName, err.Error())
		return allow, reason, err
	}

	if storageClass.AllowVolumeExpansion == nil || !*storageClass.AllowVolumeExpansion {
		reason = fmt.Sprintf("Field AllowVolumeExpansion of storageClass %s is false, Capacity expansion is not allowed", storageClassName)
		return allow, reason, nil
	}

	// If the volume name is empty, it means that it is not bound yet and can be expanded
	if pvc.Spec.VolumeName == "" {
		allow = true
		return allow, reason, err
	}

	pv, err := great.GetPv(pvc.Spec.VolumeName)
	if err != nil {
		dblog.Log.Errorf("failed to get pv %s, message: %s", pvc.Spec.VolumeName, err.Error())
		return allow, reason, err
	}

	spec := pv.Spec
	// https://kubernetes.io/zh-cn/docs/concepts/storage/storage-classes/#allow-volume-expansion
	if spec.GCEPersistentDisk == nil && spec.AWSElasticBlockStore == nil && spec.Cinder == nil && spec.Glusterfs == nil && spec.RBD == nil &&
		spec.AzureDisk == nil && spec.AzureFile == nil && spec.PortworxVolume == nil && spec.FlexVolume == nil && spec.CSI == nil {
		reason = fmt.Sprintf(" The source used by the pv bound with pvc %s/%s does not support capacity expansion", pvc.Namespace, pvc.Name)
		return allow, reason, nil
	}

	return true, "", nil

}

func (great GreatDBManager) Deletepvc(pvc *corev1.PersistentVolumeClaim) error {
	if len(pvc.Finalizers) != 0 {
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
		_, err := great.Client.KubeClientset.CoreV1().PersistentVolumeClaims(pvc.Namespace).Patch(
			context.TODO(), pvc.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of pvc %s/%s,message: %s", pvc.Namespace, pvc.Name, err.Error())
			return err
		}
	}

	err := great.Client.KubeClientset.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(context.TODO(), pvc.Name, metav1.DeleteOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}

		dblog.Log.Reason(err).Errorf("failed to clean pvc %s/%s", pvc.Name, pvc.Namespace)
		return err
	}

	return nil
}

func (great GreatDBManager) DeleteFinalizers(ns, pvcName string) error {

	pvc, err := great.Lister.PvcLister.PersistentVolumeClaims(ns).Get(pvcName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if len(pvc.Finalizers) == 0 {

		return nil
	}
	patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
	_, err = great.Client.KubeClientset.CoreV1().PersistentVolumeClaims(ns).Patch(
		context.TODO(), pvcName, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		dblog.Log.Errorf("failed to delete finalizers of pvc %s/%s,message: %s", ns, pvcName, err.Error())
		return err
	}

	return nil
}

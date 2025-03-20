package pod

import (
	"context"
	"encoding/json"
	"fmt"

	"greatdb.com/greatdb-operator/api/v1"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager/greatdbpaxos/core"
	"greatdb.com/greatdb-operator/internal/utils/log"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ReadAndWriteManager struct {
	Client client.Client
}

func (r *ReadAndWriteManager) Sync(ctx context.Context, cluster *v1.GreatDBPaxos) error {

	if err := r.updateRole(ctx, cluster); err != nil {
		return err
	}

	return nil
}

func (r *ReadAndWriteManager) updateRole(ctx context.Context, cluster *v1.GreatDBPaxos) error {

	ns := cluster.Namespace
	diag := core.DiagnoseCluster(ctx, cluster, r.Client)

	for _, ins := range diag.AllInstance {
		if err := r.updatePod(ctx, ns, ins.PodIns.Name, ins.Role, ins.State, cluster); err != nil {
			return err
		}
	}
	return nil

}

func (r *ReadAndWriteManager) updatePod(ctx context.Context, ns, podName string, role string, status v1.MemberConditionType, cluster *v1.GreatDBPaxos) error {

	var pod = &corev1.Pod{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: ns, Name: podName}, pod)
	if err != nil {
		log.Log.Reason(err).Errorf("failed to lister pod %s/%s", ns, podName)
		if k8serrors.IsNotFound(err) {
			return nil
		}

		return err
	}

	up, data := r.updateLabels(pod, cluster, role, status)
	if !up {
		return nil
	}

	err = r.Client.Patch(ctx, pod, client.RawPatch(types.JSONPatchType, []byte(data)))
	if err != nil {
		log.Log.Reason(err).Errorf("failed to update label of pod %s/%s", pod.Namespace, pod.Name)
		return err
	}

	return nil

}

func (r *ReadAndWriteManager) updateLabels(pod *corev1.Pod, cluster *v1.GreatDBPaxos, role string, status v1.MemberConditionType) (bool, string) {
	needUpdate := false
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	newLabel := resourcemanager.MergeLabels(pod.Labels)

	labels := resourcemanager.MergeLabels(cluster.Spec.Labels, r.GetLabels(cluster.Name, role))
	ready := resourcemanager.AppKubeServiceNotReady
	if status == v1.MemberStatusOnline {
		ready = resourcemanager.AppKubeServiceReady
	}
	labels[resourcemanager.AppKubeServiceReadyLabelKey] = ready

	for key, value := range labels {
		if v, ok := newLabel[key]; !ok || v != value {
			newLabel[key] = value
			needUpdate = true
		}
	}
	patch := ""
	if needUpdate {
		data, _ := json.Marshal(newLabel)
		patch = fmt.Sprintf(`[{"op":"add","path":"/metadata/labels","value":%s}]`, data)
	}

	return needUpdate, patch
}

// GetLabels  Return to the default label settings
func (r *ReadAndWriteManager) GetLabels(clusterName, role string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resourcemanager.AppKubeNameLabelKey] = resourcemanager.AppKubeNameLabelValue
	labels[resourcemanager.AppkubeManagedByLabelKey] = resourcemanager.AppkubeManagedByLabelValue
	labels[resourcemanager.AppKubeInstanceLabelKey] = clusterName
	labels[resourcemanager.AppKubeGreatDBRoleLabelKey] = role
	return

}

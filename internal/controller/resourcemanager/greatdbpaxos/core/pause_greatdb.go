package core

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	v1alpha1 "greatdb.com/greatdb-operator/api/v1"
)

// pauseGreatdb Whether to pause the return instance
func (g *GreatDBPaxosManager) pauseGreatDB(ctx context.Context, cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (bool, error) {

	if cluster.Spec.Pause == nil {
		cluster.Spec.Pause = &v1alpha1.PauseGreatDB{}
	}

	if !NeedPause(cluster, member) {
		return false, nil
	}

	return g.pauseInstance(ctx, cluster, member)

}

// Pause successfully returns true
func (g *GreatDBPaxosManager) pauseInstance(ctx context.Context, cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (bool, error) {
	var pod = &corev1.Pod{}
	err := g.Client.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: member.Name}, pod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}

	if !pod.DeletionTimestamp.IsZero() {
		return true, err
	}

	err = g.deletePod(ctx, pod)
	if err != nil {
		return false, err
	}
	return true, nil

}

func NeedPause(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) bool {

	if !cluster.Spec.Pause.Enable {
		return false
	}

	if cluster.Spec.Pause.Mode == v1alpha1.ClusterPause {
		return true
	}

	for _, ins := range cluster.Spec.Pause.Instances {
		if ins == member.Name {
			return true
		}

	}
	return false
}

func (*GreatDBPaxosManager) GetRunningMember(cluster *v1alpha1.GreatDBPaxos) int {

	num := 0
	for _, member := range cluster.Status.Member {
		if member.Type == v1alpha1.MemberStatusPause {
			continue
		}
		num++
	}

	return num
}

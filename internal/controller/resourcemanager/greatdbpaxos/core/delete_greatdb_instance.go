package core

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"greatdb.com/greatdb-operator/api/v1"
	dblog "greatdb.com/greatdb-operator/internal/utils/log"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// pauseGreatdb Whether to pause the return instance
func (g *GreatDBPaxosManager) deleteGreatDB(ctx context.Context, cluster *v1.GreatDBPaxos, member v1.MemberCondition) (bool, error) {

	if cluster.Spec.Delete == nil {
		cluster.Spec.Delete = &v1.DeleteInstance{}
	}

	return g.deleteInstance(ctx, cluster, member)

}

// Pause successfully returns true
func (g *GreatDBPaxosManager) deleteInstance(ctx context.Context, cluster *v1.GreatDBPaxos, member v1.MemberCondition) (bool, error) {

	needDel := false
	for _, insName := range cluster.Spec.Delete.Instances {
		if insName == member.Name {
			needDel = true
			break
		}
	}

	if !needDel {
		return false, nil
	}
	var pod = &corev1.Pod{}
	err := g.Client.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: member.Name}, pod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, err

		}
		// dblog.Log.Reason(err).Error("")
		dblog.Log.Reason(err).Error("failed to lister pod")
	}

	err = g.deletePod(ctx, pod)
	if err != nil {
		return false, err
	}

	if cluster.Spec.Delete.CleanPvc {
		var pvc = &corev1.PersistentVolumeClaim{}
		err = g.Client.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: member.PvcName}, pvc)
		if err != nil {
			if !k8serrors.IsNotFound(err) {
				return false, err
			}
		} else {
			err = g.Deletepvc(ctx, pvc)
			if err != nil {
				return false, err
			}
		}

	}

	// remove
	g.removeMember(cluster, member.Name)

	return true, nil

}

func (_ *GreatDBPaxosManager) removeMember(cluster *v1.GreatDBPaxos, name string) {
	memberList := make([]v1.MemberCondition, 0)

	for _, member := range cluster.Status.Member {
		if member.Name == name {
			cluster.Status.CurrentInstances -= 1
			continue
		}
		memberList = append(memberList, member)
	}
	cluster.Status.Member = memberList
}

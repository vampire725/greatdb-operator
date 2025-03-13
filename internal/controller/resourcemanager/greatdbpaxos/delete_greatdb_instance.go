package greatdbpaxos

import (
	v1alpha1 "greatdb.com/greatdb-operator/api/v1"
	dblog "greatdb.com/greatdb-operator/internal/utils/log"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// pauseGreatdb Whether to pause the return instance
func (great GreatDBManager) deleteGreatDB(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (bool, error) {

	if cluster.Spec.Delete == nil {
		cluster.Spec.Delete = &v1alpha1.DeleteInstance{}
	}

	return great.deleteInstance(cluster, member)

}

// Pause successfully returns true
func (great GreatDBManager) deleteInstance(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) (bool, error) {

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

	pod, err := great.Lister.PodLister.Pods(cluster.Namespace).Get(member.Name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, err

		}
		// dblog.Log.Reason(err).Error("")
		dblog.Log.Reason(err).Error("failed to lister pod")
	}

	err = great.deletePod(pod)
	if err != nil {
		return false, err
	}

	if cluster.Spec.Delete.CleanPvc {
		pvc, err := great.Lister.PvcLister.PersistentVolumeClaims(cluster.Namespace).Get(member.PvcName)
		if err != nil {
			if !k8serrors.IsNotFound(err) {
				return false, err
			}
		} else {
			err = great.Deletepvc(pvc)
			if err != nil {
				return false, err
			}
		}

	}

	// remove
	great.removeMember(cluster, member.Name)

	return true, nil

}

func (GreatDBManager) removeMember(cluster *v1alpha1.GreatDBPaxos, name string) {
	memberList := make([]v1alpha1.MemberCondition, 0)

	for _, member := range cluster.Status.Member {
		if member.Name == name {
			cluster.Status.CurrentInstances -= 1
			continue
		}
		memberList = append(memberList, member)
	}
	cluster.Status.Member = memberList
}

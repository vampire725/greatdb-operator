package greatdbpaxos

import (
	"fmt"
	"time"

	v1alpha1 "greatdb.com/greatdb-operator/api/v1"
	"greatdb.com/greatdb-operator/internal/config"
	"greatdb.com/greatdb-operator/internal/utils/log"
	"greatdb.com/greatdb-operator/internal/utils/tools"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (great GreatDBManager) failover(cluster *v1alpha1.GreatDBPaxos) error {
	// Fault prevention
	if err := great.prevention(cluster); err != nil {
		return err
	}

	if err := great.faultRecovery(cluster); err != nil {
		return err
	}
	return nil

}

func (great GreatDBManager) prevention(cluster *v1alpha1.GreatDBPaxos) error {
	pdbName := great.getPDBName(cluster)
	if config.ApiVersion.PDB == "policy/v1" {
		pdb, err := great.Lister.PDBV1Lister.PodDisruptionBudgets(cluster.Namespace).Get(pdbName)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				err := great.createV1PDB(cluster)
				if err != nil {
					return err
				}
				return nil
			}
			log.Log.Reason(err).Errorf("failed to lister pdb %s/%s", cluster.Namespace, pdbName)
		}
		newPDB := pdb.DeepCopy()
		return great.updateV1PDB(cluster, newPDB)

	} else {

		pdb, err := great.Lister.PDBLister.PodDisruptionBudgets(cluster.Namespace).Get(pdbName)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				err := great.createPDB(cluster)
				if err != nil {
					return err
				}
				return nil
			}
			log.Log.Reason(err).Errorf("failed to lister pdb %s/%s", cluster.Namespace, pdbName)
		}
		newPDB := pdb.DeepCopy()
		return great.updatePDB(cluster, newPDB)
	}

}

func (great GreatDBManager) faultRecovery(cluster *v1alpha1.GreatDBPaxos) error {

	if !*cluster.Spec.FailOver.Enable {
		return nil
	}

	failedIns := great.stateTransition(cluster)

	switch cluster.Status.Phase {
	case v1alpha1.GreatDBPaxosReady:
		if failedIns > 0 {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosFailover, "")
		}

	case v1alpha1.GreatDBPaxosFailover:

		if cluster.Spec.FailOver.MaxInstance < failedIns {
			cluster.Status.TargetInstances = cluster.Status.Instances + cluster.Spec.FailOver.MaxInstance
		} else {
			cluster.Status.TargetInstances = cluster.Status.Instances + int32(failedIns)
		}

		if cluster.Status.ReadyInstances == cluster.Status.Instances {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosFailoverSucceeded, "")
		}

	case v1alpha1.GreatDBPaxosFailoverSucceeded:
		if cluster.Status.TargetInstances < cluster.Status.Instances+int32(failedIns) {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosFailover, "")
			cluster.Status.TargetInstances = cluster.Status.Instances + int32(failedIns)
			if cluster.Spec.FailOver.MaxInstance < failedIns {
				cluster.Status.TargetInstances = cluster.Status.Instances + cluster.Spec.FailOver.MaxInstance
			}
			return nil
		}

		if *cluster.Spec.FailOver.AutoScaleIn {
			if cluster.Spec.FailOver.MaxInstance < failedIns {
				cluster.Status.TargetInstances = cluster.Status.Instances + cluster.Spec.FailOver.MaxInstance
			} else {
				cluster.Status.TargetInstances = cluster.Status.Instances + int32(failedIns)
			}
		}

		if cluster.Status.ReadyInstances != cluster.Status.TargetInstances {
			return nil
		}

		if *cluster.Spec.FailOver.AutoScaleIn {
			cluster.Status.TargetInstances = cluster.Status.Instances
		}

	}

	return nil

}

// stateTransition  Check the instance status to determine whether the failover period has been reached
// Return the number of failed instances and normal instances
func (great GreatDBManager) stateTransition(cluster *v1alpha1.GreatDBPaxos) (failedIns int32) {
	period, err := tools.StringToDuration(cluster.Spec.FailOver.Period)
	if err != nil {
		period = time.Minute * 10
	}
	now := metav1.Now()

	for index, mem := range cluster.Status.Member {
		if mem.Type == v1alpha1.MemberStatusFailure {
			failedIns++
			continue
		}
		if mem.Type == v1alpha1.MemberStatusError || mem.Type == v1alpha1.MemberStatusOffline ||
			mem.Type == v1alpha1.MemberStatusUnmanaged || mem.Type == v1alpha1.MemberStatusUnknown || (mem.Type == v1alpha1.MemberStatusPending && mem.JoinCluster) {
			if now.Sub(mem.LastTransitionTime.Time) > period {

				cluster.Status.Member[index].LastUpdateTime = now
				cluster.Status.Member[index].LastTransitionTime = now
				cluster.Status.Member[index].Type = v1alpha1.MemberStatusFailure
				cluster.Status.Member[index].Message = fmt.Sprintf("The instance is in state %s for more than the failover period %s", mem.Type, cluster.Spec.FailOver.Period)
				failedIns++
			}

		}

	}
	return
}

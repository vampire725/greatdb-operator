package core

import (
	"context"
	"fmt"
	"time"

	policyV1 "k8s.io/api/policy/v1"
	"k8s.io/api/policy/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "greatdb.com/greatdb-operator/api/v1"
	"greatdb.com/greatdb-operator/internal/config"
	"greatdb.com/greatdb-operator/internal/utils/log"
	"greatdb.com/greatdb-operator/internal/utils/tools"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (g *GreatDBPaxosManager) failOver(ctx context.Context, cluster *v1alpha1.GreatDBPaxos) error {
	// Fault prevention
	if err := g.prevention(ctx, cluster); err != nil {
		return err
	}

	if err := g.faultRecovery(cluster); err != nil {
		return err
	}
	return nil

}

func (g *GreatDBPaxosManager) prevention(ctx context.Context, cluster *v1alpha1.GreatDBPaxos) error {
	pdbName := g.getPDBName(cluster)
	if config.ApiVersion.PDB == "policy/v1" {
		var pdb = &policyV1.PodDisruptionBudget{}
		err := g.Client.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: pdbName}, pdb)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				err := g.createV1PDB(ctx, cluster)
				if err != nil {
					return err
				}
				return nil
			}
			log.Log.Reason(err).Errorf("failed to lister pdb %s/%s", cluster.Namespace, pdbName)
		}
		newPDB := pdb.DeepCopy()
		return g.updateV1PDB(ctx, cluster, newPDB)

	} else {

		var pdb = &v1beta1.PodDisruptionBudget{}
		err := g.Client.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: pdbName}, pdb)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				err := g.createPDB(ctx, cluster)
				if err != nil {
					return err
				}
				return nil
			}
			log.Log.Reason(err).Errorf("failed to lister pdb %s/%s", cluster.Namespace, pdbName)
		}
		newPDB := pdb.DeepCopy()
		return g.updatePDB(ctx, cluster, newPDB)
	}

}

func (g *GreatDBPaxosManager) faultRecovery(cluster *v1alpha1.GreatDBPaxos) error {

	if !*cluster.Spec.FailOver.Enable {
		return nil
	}

	failedIns := g.stateTransition(cluster)

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

// stateTransition  Check the instance status to determine whether the failOver period has been reached
// Return the number of failed instances and normal instances
func (g *GreatDBPaxosManager) stateTransition(cluster *v1alpha1.GreatDBPaxos) (failedIns int32) {
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
				cluster.Status.Member[index].Message = fmt.Sprintf("The instance is in state %s for more than the failOver period %s", mem.Type, cluster.Spec.FailOver.Period)
				failedIns++
			}

		}

	}
	return
}

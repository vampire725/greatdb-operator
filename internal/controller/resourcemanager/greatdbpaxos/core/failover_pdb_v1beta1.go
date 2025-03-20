package core

import (
	"context"
	"fmt"
	"reflect"

	v1alpha1 "greatdb.com/greatdb-operator/api/v1"
	resources "greatdb.com/greatdb-operator/internal/controller/resourcemanager"
	"greatdb.com/greatdb-operator/internal/utils/log"

	"k8s.io/api/policy/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (g *GreatDBPaxosManager) createPDB(ctx context.Context, cluster *v1alpha1.GreatDBPaxos) error {

	pdb := g.newPDB(cluster)
	err := g.Client.Create(ctx, pdb)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil
		}
		log.Log.Reason(err).Errorf("failed to create pdb %s/%s", pdb.Namespace, pdb.Name)
		return err
	}

	return nil
}

func (g *GreatDBPaxosManager) newPDB(cluster *v1alpha1.GreatDBPaxos) *v1beta1.PodDisruptionBudget {

	labels := g.GetLabels(cluster.Name)
	pdbSpec := v1beta1.PodDisruptionBudgetSpec{

		MaxUnavailable: cluster.Spec.FailOver.MaxUnavailable,
		Selector:       metav1.SetAsLabelSelector(labels),
	}
	pdbName := g.getPDBName(cluster)

	if pdbSpec.Selector == nil {
		pdbSpec.Selector = metav1.SetAsLabelSelector(labels)
	}

	pdb := &v1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:            pdbName,
			Namespace:       cluster.Namespace,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)},
		},
		Spec: pdbSpec,
	}

	return pdb
}

func (g *GreatDBPaxosManager) updatePDB(ctx context.Context, cluster *v1alpha1.GreatDBPaxos, pdb *v1beta1.PodDisruptionBudget) error {
	labels := g.GetLabels(cluster.Name)
	pdbSpec := v1beta1.PodDisruptionBudgetSpec{
		MaxUnavailable: cluster.Spec.FailOver.MaxUnavailable,
		Selector:       metav1.SetAsLabelSelector(labels),
	}

	if reflect.DeepEqual(pdbSpec, pdb.Spec) {
		return nil
	}
	pdb.Spec = pdbSpec
	err := g.Client.Update(ctx, pdb)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		log.Log.Reason(err).Errorf("failed to update pdb %s/%s", pdb.Namespace, pdb.Name)
		return err
	}

	return nil
}

func (*GreatDBPaxosManager) getPDBName(cluster *v1alpha1.GreatDBPaxos) string {
	return fmt.Sprintf("%s%s", cluster.Name, resources.ComponentGreatDBSuffix)
}

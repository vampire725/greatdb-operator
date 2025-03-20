package core

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "greatdb.com/greatdb-operator/api/v1"
	resources "greatdb.com/greatdb-operator/internal/controller/resourcemanager"
	dblog "greatdb.com/greatdb-operator/internal/utils/log"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
)

func (g *GreatDBPaxosManager) restartGreatDB(ctx context.Context, cluster *v1alpha1.GreatDBPaxos, podIns *corev1.Pod) (err error) {

	if cluster.Status.Phase != v1alpha1.GreatDBPaxosReady && cluster.Status.Phase != v1alpha1.GreatDBPaxosRestart {
		return nil
	}

	if cluster.Spec.Restart == nil {
		cluster.Spec.Restart = &v1alpha1.RestartGreatDB{}
	}

	if cluster.Status.RestartMember.Restarting == nil {
		cluster.Status.RestartMember.Restarting = make(map[string]string, 0)
	}
	if cluster.Status.RestartMember.Restarted == nil {
		cluster.Status.RestartMember.Restarted = make(map[string]string, 0)
	}

	if !cluster.Spec.Restart.Enable && len(cluster.Status.RestartMember.Restarting) == 0 {
		return
	}

	if cluster.Spec.Restart.Mode == v1alpha1.ClusterRestart {
		err = g.restartCluster(ctx, cluster, podIns)
		if err != nil {
			return err
		}
	} else {
		err = g.restartInstance(ctx, cluster, podIns)
		if err != nil {
			return err
		}
	}

	if len(cluster.Status.RestartMember.Restarting) > 0 {
		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosRestart, "")
	}

	return nil
}

func (g *GreatDBPaxosManager) restartInstance(ctx context.Context, cluster *v1alpha1.GreatDBPaxos, podIns *corev1.Pod) error {

	if value, ok := cluster.Status.RestartMember.Restarting[podIns.Name]; ok {
		// Restarting requires at least 30 seconds before continuing to determine
		t := resources.StringToTime(value)
		if resources.GetNowTime().Sub(t) < time.Second*30 {
			return nil
		}
		for _, member := range cluster.Status.Member {
			if member.Name == podIns.Name && member.Type == v1alpha1.MemberStatusOnline &&
				(t.Sub(member.LastTransitionTime.Time) < 0 || t.Sub(podIns.CreationTimestamp.Time) < 0) {
				cluster.Status.RestartMember.Restarted[podIns.Name] = resources.GetNowTimeToString()
				delete(cluster.Status.RestartMember.Restarting, podIns.Name)
				break
			}
		}
		return nil
	}

	needRestart := false
	endRestart := true
	for _, ins := range cluster.Spec.Restart.Instances {
		if ins == podIns.Name {
			needRestart = true
		}
		if _, ok := cluster.Status.RestartMember.Restarted[ins]; !ok {
			endRestart = false
		}
	}

	if endRestart && len(cluster.Status.RestartMember.Restarting) == 0 && len(cluster.Status.RestartMember.Restarted) != 0 {
		cluster.Status.RestartMember.Restarted = make(map[string]string, 0)
		cluster.Status.RestartMember.Restarting = make(map[string]string, 0)
		cluster.Spec.Restart.Enable = false
		return nil
	}

	if !needRestart {
		return nil
	}

	if _, ok := cluster.Status.RestartMember.Restarted[podIns.Name]; ok {
		return nil
	}
	if !podIns.DeletionTimestamp.IsZero() {
		cluster.Status.RestartMember.Restarting[podIns.Name] = resources.GetNowTimeToString()
		return nil
	}

	// Restart according to strategy
	switch cluster.Spec.Restart.Strategy {
	case v1alpha1.AllRestart:

		err := g.deletePod(ctx, podIns)

		if err != nil {
			return err
		}

	case v1alpha1.RollingRestart:
		if len(cluster.Status.RestartMember.Restarting) > 0 {
			return nil
		}

		err := g.deletePod(ctx, podIns)

		if err != nil {
			return err
		}

	}
	cluster.Status.RestartMember.Restarting[podIns.Name] = resources.GetNowTimeToString()

	return nil

}

func (g *GreatDBPaxosManager) restartCluster(ctx context.Context, cluster *v1alpha1.GreatDBPaxos, podIns *corev1.Pod) error {

	if value, ok := cluster.Status.RestartMember.Restarting[podIns.Name]; ok {
		// Restarting requires at least 30 seconds before continuing to determine
		t := resources.StringToTime(value)

		if resources.GetNowTime().Sub(t) < time.Second*30 {
			return nil
		}
		for _, member := range cluster.Status.Member {

			if member.Name == podIns.Name && member.Type == v1alpha1.MemberStatusOnline && t.Sub(member.LastTransitionTime.Time) < 0 {
				cluster.Status.RestartMember.Restarted[podIns.Name] = resources.GetNowTimeToString()

				delete(cluster.Status.RestartMember.Restarting, podIns.Name)
				break
			}
		}
		return nil
	}

	if g.restartClusterEnds(cluster) {
		cluster.Status.RestartMember.Restarted = make(map[string]string, 0)
		cluster.Status.RestartMember.Restarting = make(map[string]string, 0)
		cluster.Spec.Restart.Enable = false
		return nil
	}
	if _, ok := cluster.Status.RestartMember.Restarted[podIns.Name]; ok {
		return nil
	}

	if !podIns.DeletionTimestamp.IsZero() {
		cluster.Status.RestartMember.Restarting[podIns.Name] = resources.GetNowTimeToString()
		return nil
	}

	// Restart according to strategy
	switch cluster.Spec.Restart.Strategy {
	case v1alpha1.AllRestart:
		err := g.deletePod(ctx, podIns)
		if err != nil {
			return err
		}

	case v1alpha1.RollingRestart:
		if len(cluster.Status.RestartMember.Restarting) > 0 {
			return nil
		}
		err := g.deletePod(ctx, podIns)
		if err != nil {
			return err
		}

	}
	cluster.Status.RestartMember.Restarting[podIns.Name] = resources.GetNowTimeToString()

	return nil

}

func (g *GreatDBPaxosManager) deletePod(ctx context.Context, pod *corev1.Pod) error {

	if len(pod.Finalizers) != 0 {

		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`
		err := g.Client.Patch(ctx, pod, client.RawPatch(types.JSONPatchType, []byte(patch)))
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil
			}
			dblog.Log.Reason(err).Errorf("failed to delete finalizers of pods  %s/%s,", pod.Namespace, pod.Name)
			return err
		}

	}

	if !pod.DeletionTimestamp.IsZero() {
		return nil
	}

	err := g.Client.Delete(ctx, pod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		dblog.Log.Reason(err).Errorf("failed to restart  pods  %s/%s,", pod.Namespace, pod.Name)
		return err
	}

	return nil

}

func (*GreatDBPaxosManager) restartClusterEnds(cluster *v1alpha1.GreatDBPaxos) bool {
	end := true
	for _, member := range cluster.Status.Member {
		if member.Type == v1alpha1.MemberStatusPause {
			continue
		}

		if _, ok := cluster.Status.RestartMember.Restarted[member.Name]; !ok {
			end = false
		}
	}

	return end
}

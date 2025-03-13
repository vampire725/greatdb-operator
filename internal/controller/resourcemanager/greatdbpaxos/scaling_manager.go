package greatdbpaxos

import (
	"fmt"
	"strings"

	v1alpha1 "greatdb.com/greatdb-operator/api/v1"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager/internal"
	dblog "greatdb.com/greatdb-operator/internal/utils/log"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

func (great GreatDBManager) Scale(cluster *v1alpha1.GreatDBPaxos) error {

	if cluster.Status.Phase != v1alpha1.GreatDBPaxosReady && cluster.Status.Phase != v1alpha1.GreatDBPaxosScaleIn &&
		cluster.Status.Phase != v1alpha1.GreatDBPaxosScaleOut && cluster.Status.Phase != v1alpha1.GreatDBPaxosFailover {
		return nil
	}

	if cluster.Status.TargetInstances != cluster.Spec.Instances && (cluster.Status.Phase == v1alpha1.GreatDBPaxosReady ||
		cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleOut || cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleIn) {
		cluster.Status.TargetInstances = cluster.Spec.Instances
	}

	if err := great.ScaleOut(cluster); err != nil {
		return err
	}

	if err := great.ScaleIn(cluster); err != nil {
		return err
	}

	return nil

}

func (great GreatDBManager) ScaleOut(cluster *v1alpha1.GreatDBPaxos) error {

	if cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleIn {
		return nil
	}

	// Only one node can be expanded at a time
	if cluster.Status.TargetInstances > cluster.Status.CurrentInstances {
		//determine whether the instance has completed expansion (joining the cluster)
		member := great.GetMinOrMaxIndexMember(cluster, true)
		// Waiting for the expansion of the previous instance to end
		if member.Type != v1alpha1.MemberStatusOnline && cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleOut {
			return nil
		}

		if cluster.Status.Phase != v1alpha1.GreatDBPaxosFailover && cluster.Status.Phase != v1alpha1.GreatDBPaxosScaleOut {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosScaleOut, "")
		}

		num := len(cluster.Status.Member)

		if num >= int(cluster.Status.TargetInstances) {
			return nil
		}

		index := GetNextIndex(cluster.Status.Member)
		name := fmt.Sprintf("%s%s-%d", cluster.Name, resourcemanager.ComponentGreatDBSuffix, index)
		cluster.Status.Member = append(cluster.Status.Member, v1alpha1.MemberCondition{
			Name:       name,
			Index:      index,
			CreateType: v1alpha1.ScaleCreateMember,
			PvcName:    name,
		})
		cluster.Status.CurrentInstances += 1

		groupSeed := ""
		hosts := great.getGreatdbServiceClientUri(cluster)

		for _, host := range hosts {
			groupSeed += fmt.Sprintf("%s:%d,", host, resourcemanager.GroupPort)
		}
		groupSeed = strings.TrimSuffix(groupSeed, ",")

		return great.SetVariableGroupSeeds(cluster, hosts, groupSeed)

	} else {
		member := great.GetMinOrMaxIndexMember(cluster, true)
		if cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleOut && member.Type == v1alpha1.MemberStatusOnline {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		}

	}

	return nil

}

func (great GreatDBManager) ScaleIn(cluster *v1alpha1.GreatDBPaxos) error {
	// Only one node can be expanded at a time
	if cluster.Status.TargetInstances < cluster.Status.CurrentInstances {
		if cluster.Status.Phase != v1alpha1.GreatDBPaxosFailover && cluster.Status.Phase != v1alpha1.GreatDBPaxosScaleIn {
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosScaleIn, "")
		}
		num := len(cluster.Status.Member)

		if num < int(cluster.Status.TargetInstances) {
			return nil
		}
		// Waiting for the previous instance to shrink to an end
		if cluster.Status.ScaleInMember != "" {
			_, err := great.Lister.PodLister.Pods(cluster.Namespace).Get(cluster.Status.ScaleInMember)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					cluster.Status.ScaleInMember = ""
					return nil
				}
				dblog.Log.Reason(err).Errorf("faield to get lister pods")
				return err
			}
			cluster.Status.ScaleInMember = ""
		}
		cluster.Status.CurrentInstances -= 1

		member := great.getScaleInMember(cluster)
		great.ScaleInMember(cluster, member.Name)

		_ = great.stopGroupReplication(cluster, member)

		err := great.DeleteFinalizers(cluster.Namespace, member.PvcName)
		if err != nil {
			return err
		}

		pod, err := great.Lister.PodLister.Pods(cluster.Namespace).Get(member.Name)

		if err != nil && !k8serrors.IsNotFound(err) {

			dblog.Log.Reason(err).Error("failed to lister pod")
			return err
		}
		err = great.deletePod(pod)
		if err != nil {
			if !k8serrors.IsNotFound(err) {
				dblog.Log.Reason(err).Error("failed to delete pod")
				return err
			}
		}

		cluster.Status.ScaleInMember = member.Name

		groupSeed := ""
		hosts := great.getGreatdbServiceClientUri(cluster)

		for _, host := range hosts {
			groupSeed += fmt.Sprintf("%s:%d,", host, resourcemanager.GroupPort)
		}
		groupSeed = strings.TrimSuffix(groupSeed, ",")

		return great.SetVariableGroupSeeds(cluster, hosts, groupSeed)

	} else {
		if cluster.Status.Phase == v1alpha1.GreatDBPaxosScaleIn {
			cluster.Status.ScaleInMember = ""
			UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosReady, "")
		}
	}

	return nil

}

func (great GreatDBManager) getGreatdbServiceClientUri(cluster *v1alpha1.GreatDBPaxos) (uris []string) {

	for _, member := range cluster.Status.Member {

		if member.Address != "" {
			uris = append(uris, member.Address)
			continue
		}
		svcName := cluster.Name + resourcemanager.ComponentGreatDBSuffix
		host := fmt.Sprintf("%s.%s.%s.svc.%s", member.Name, svcName, cluster.Namespace, cluster.Spec.ClusterDomain)
		// TODO Debug
		// host := resources.GetInstanceFQDN(cluster.Name, member.Name, cluster.Namespace, cluster.Spec.ClusterDomain)
		uris = append(uris, host)
	}

	return uris
}

func (great GreatDBManager) SetVariableGroupSeeds(cluster *v1alpha1.GreatDBPaxos, hostList []string, groupSeed string) error {

	sql := fmt.Sprintf("SET GLOBAL group_replication_group_seeds='%s'", groupSeed)
	client := internal.NewDBClient()
	user, password := resourcemanager.GetClusterUser(cluster)
	for _, host := range hostList {

		err := client.Connect(user, password, host, int(cluster.Spec.Port), "mysql")
		if err != nil {
			dblog.Log.Reason(err).Error("failed to connect mysql")
			// TODO The problem of not being able to set up a single node yet to be resolved, resulting in the failure of the expansion node to join
			continue
		}

		err = client.Exec(sql)
		if err != nil {
			client.Close()
			dblog.Log.Reason(err).Errorf("failed to set variable %s on node group_replication_group_seeds", host)
			return err
		}
		client.Close()
	}
	return nil

}

func (great GreatDBManager) getScaleInMember(cluster *v1alpha1.GreatDBPaxos) v1alpha1.MemberCondition {

	var member v1alpha1.MemberCondition
	switch cluster.Spec.Scaling.ScaleIn.Strategy {
	case v1alpha1.ScaleInStrategyDefine:
		exist := false
		index := 0
		for i, name := range cluster.Spec.Scaling.ScaleIn.Instance {

			for _, mem := range cluster.Status.Member {
				if mem.Name == name {
					member = mem
					exist = true
					break
				}
			}
			index = i
			if exist {
				break
			}

		}
		// If the set instance does not exist, return the last one
		if !exist {
			num := len(cluster.Status.Member)
			member = cluster.Status.Member[num-1]
		} else {

			cluster.Spec.Scaling.ScaleIn.Instance = cluster.Spec.Scaling.ScaleIn.Instance[index+1:]
		}

	case v1alpha1.ScaleInStrategyFault:
		maxPriority := 0
		for _, mem := range cluster.Status.Member {

			if mem.Type.ScaleInPriority() > maxPriority || member.Name == "" {
				maxPriority = mem.Type.ScaleInPriority()
				member = mem
				continue
			}
		}

		if maxPriority == 0 {
			num := len(cluster.Status.Member)
			member = cluster.Status.Member[num-1]
		}

	default:
		num := len(cluster.Status.Member)
		member = cluster.Status.Member[num-1]

	}

	return member

}

func (great GreatDBManager) ScaleInMember(cluster *v1alpha1.GreatDBPaxos, name string) {

	index := 0
	exist := false
	for i, member := range cluster.Status.Member {

		if member.Name == name {
			exist = true
			index = i
		}
	}
	if !exist {
		return
	}

	cluster.Status.Member = append(cluster.Status.Member[:index], cluster.Status.Member[index+1:]...)

}

func (great GreatDBManager) GetMinOrMaxIndexMember(cluster *v1alpha1.GreatDBPaxos, max bool) v1alpha1.MemberCondition {

	index := 0

	var member v1alpha1.MemberCondition
	for _, m := range cluster.Status.Member {
		if max {
			if m.Index >= index {
				member = m
			}
		} else {
			if m.Index <= index {
				member = m
			}
		}

	}
	return member

}

func (great GreatDBManager) stopGroupReplication(cluster *v1alpha1.GreatDBPaxos, member v1alpha1.MemberCondition) error {
	client := internal.NewDBClient()
	user, pwd := resourcemanager.GetClusterUser(cluster)
	host := resourcemanager.GetInstanceFQDN(cluster.Name, member.Name, cluster.Namespace, cluster.GetClusterDomain())
	port := int(cluster.Spec.Port)
	err := client.Connect(user, pwd, host, port, "mysql")
	if err != nil {
		dblog.Log.Reason(err).Errorf("%s/%s.%s", cluster.Namespace, cluster.Name, member.Name)
		return err
	}
	defer client.Close()

	err = client.Exec("stop group_replication;")
	if err != nil {
		dblog.Log.Reason(err).Errorf("Failed to execute SQL statement %s/%s.%s", cluster.Namespace, cluster.Name, member.Name)
		return err
	}

	return err
}

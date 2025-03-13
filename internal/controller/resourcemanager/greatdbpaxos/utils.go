package greatdbpaxos

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "greatdb.com/greatdb-operator/api/v1"
	resources "greatdb.com/greatdb-operator/internal/controller/resourcemanager"
	"greatdb.com/greatdb-operator/internal/controller/resourcemanager/internal"
)

func SplitImage(imageName string) (repo, tag string) {

	match, _ := regexp.MatchString("^[a-zA-Z0-9./:-]+(/[a-zA-Z0-9./:-]+)?(:[a-zA-Z0-9._-]+)?$", imageName)

	if match {

		r := regexp.MustCompile(`^(?P<registry>[a-zA-Z0-9.-]+(:[0-9]+)?/)?(?P<repository>[a-zA-Z0-9./-]+)(:(?P<tag>[a-zA-Z0-9._-]+))?$`)
		match := r.FindStringSubmatch(imageName)

		registry := match[r.SubexpIndex("registry")]
		repository := match[r.SubexpIndex("repository")]

		tag := match[r.SubexpIndex("tag")]

		if tag == "" {
			tag = "latest"
		}

		repo = registry + repository
		return repo, tag
	}

	return
}

func getLabels(name string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeInstanceLabelKey] = name
	return

}

func getDefaultAffinity(clusterName string) *corev1.Affinity {
	label := getLabels(clusterName)
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{

			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 1,

					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: label,
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		},
	}
}

func UpdateClusterStatusCondition(cluster *v1alpha1.GreatDBPaxos, statusType v1alpha1.GreatDBPaxosConditionType, message string) {
	if cluster.Status.Phase != statusType {
		cluster.Status.Phase = statusType
	}

	if cluster.Status.Conditions == nil {
		cluster.Status.Conditions = make([]v1alpha1.GreatDBPaxosConditions, 0)
	}

	status := v1alpha1.ConditionTrue
	exist := false
	now := metav1.Now()
	for index, cond := range cluster.Status.Conditions {
		if now.Sub(cluster.Status.Conditions[index].LastUpdateTime.Time) > time.Minute*2 {
			cluster.Status.Conditions[index].LastUpdateTime = now
		}
		if cond.Type == statusType {
			exist = true
			cluster.Status.Conditions[index].Message = message
			if cond.Status == status {
				return
			}
			cluster.Status.Conditions[index].Status = status
			cluster.Status.Conditions[index].LastUpdateTime = now
			cluster.Status.Conditions[index].LastTransitionTime = now
			continue
		}

		if cond.Status == status {
			cluster.Status.Conditions[index].LastUpdateTime = now
			cluster.Status.Conditions[index].Status = v1alpha1.ConditionFalse
		}

	}

	if !exist {
		cluster.Status.Conditions = append(cluster.Status.Conditions, v1alpha1.GreatDBPaxosConditions{
			Type:               statusType,
			Status:             status,
			Message:            message,
			LastUpdateTime:     now,
			LastTransitionTime: now,
		})

	}

}

func GetNextIndex(memberList []v1alpha1.MemberCondition) int {
	index := -1

	for _, member := range memberList {
		if member.Index > index {
			index = member.Index
		}
	}
	return index + 1
}

func GetNowTime() string {

	return time.Now().Local().Format("2006-01-02 15:04:05")
}

func StringToTime(value string) time.Time {

	n, _ := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	return n
}

func CountGtids(gtidSet string) int {
	// Return number of transactions in the GTID set
	countRange := func(r string) int {
		parts := strings.Split(r, "-")
		if len(parts) == 1 {
			return 1
		} else {
			begin, _ := strconv.Atoi(parts[0])
			end, _ := strconv.Atoi(parts[1])
			return end - begin + 1
		}
	}
	n := 0
	gtidSet = strings.ReplaceAll(gtidSet, "\n", "")
	gtidList := strings.Split(gtidSet, ",")
	for _, g := range gtidList {
		rList := strings.Split(g, ":")[1:]
		for _, r := range rList {
			n += countRange(r)
		}
	}
	return n
}

func containsUnreachableStates(clusterDiagStatus v1alpha1.ClusterDiagStatusType) bool {
	unreachableStates := []v1alpha1.ClusterDiagStatusType{
		v1alpha1.ClusterDiagStatusUnknown,
		v1alpha1.ClusterDiagStatusOnlineUncertain,
		v1alpha1.ClusterDiagStatusOfflineUncertain,
		v1alpha1.ClusterDiagStatusNoQuorumUncertain,
		v1alpha1.ClusterDiagStatusSplitBrainUncertain,
	}

	for _, us := range unreachableStates {
		if us == clusterDiagStatus {
			return true
		}
	}
	return false
}

func updateStatusCheck(cluster *v1alpha1.GreatDBPaxos) bool {
	clusterLastProbeTime := cluster.Status.LastProbeTime
	if clusterLastProbeTime.IsZero() {
		return true
	}

	for _, condition := range cluster.Status.Conditions {
		if condition.LastTransitionTime.IsZero() {
			continue
		}
		if condition.Type == v1alpha1.GreatDBPaxosRepair && condition.Status == "True" {
			return true
		}
		if metav1.Now().Sub(clusterLastProbeTime.Time) > metav1.Now().Sub(condition.LastTransitionTime.Time) {
			return true
		}
	}
	if metav1.Now().Sub(clusterLastProbeTime.Time) > time.Minute*1 {
		return true
	}
	return containsUnreachableStates(cluster.Status.DiagStatus)
}

func SubtractSlices(a, b []string) []string {

	setB := make(map[string]bool)
	for _, v := range b {
		setB[v] = true
	}
	result := []string{}
	for _, k := range a {
		if !setB[k] {
			result = append(result, k)
		}
	}
	return result

}

func GetNormalMemberSqlClient(cluster *v1alpha1.GreatDBPaxos) (internal.DBClientinterface, error) {

	user, pwd := resources.GetClusterUser(cluster)
	port := int(cluster.Spec.Port)

	client := internal.NewDBClient()

	for _, member := range cluster.Status.Member {

		if member.Type != v1alpha1.MemberStatusFree && member.Type != v1alpha1.MemberStatusOnline {
			continue
		}
		host := member.Address
		if host == "" {
			host = resources.GetInstanceFQDN(cluster.Name, member.Name, cluster.Namespace, cluster.Spec.ClusterDomain)
		}

		err := client.Connect(user, pwd, host, port, "mysql")

		if err != nil {
			return nil, err
		}
		return client, nil

	}
	return nil, fmt.Errorf("no available connections")

}

func GreatdbInstanceStatusNormal(status v1alpha1.MemberConditionType) bool {

	if status == v1alpha1.MemberStatusOnline || status == v1alpha1.MemberStatusRecovering {
		return true
	}

	return false

}

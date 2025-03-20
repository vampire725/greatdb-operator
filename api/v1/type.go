package v1

import "strings"

type UpgradeStrategyType string

const (
	AllUpgrade     UpgradeStrategyType = "all"
	RollingUpgrade UpgradeStrategyType = "rollingUpgrade"
)

type PauseModeType string

const (
	ClusterPause PauseModeType = "cluster"
	InsPause     PauseModeType = "ins"
)

type RestartModeType string

const (
	ClusterRestart RestartModeType = "cluster"
	InsRestart     RestartModeType = "ins"
)

type RestartStrategyType string

const (
	AllRestart     RestartStrategyType = "all"
	RollingRestart RestartStrategyType = "rolling"
)

type CandidateDiagStatus string

const (
	CandidateDiagStatusUnknown CandidateDiagStatus = "UNKNOWN"

	// CandidateDiagStatusMember MEMBER Instance is already a member of the cluster
	CandidateDiagStatusMember CandidateDiagStatus = "MEMBER"

	// CandidateDiagStatusRejoinAble REJOIN ABLE Instance is a member of the cluster but can rejoin it
	CandidateDiagStatusRejoinAble CandidateDiagStatus = "REJOINABLE"

	// CandidateDiagStatusJoinAble JOIN ABLE Instance is not yet a member of the cluster but can join it
	CandidateDiagStatusJoinAble CandidateDiagStatus = "JOINABLE"

	// CandidateDiagStatusBroken BROKEN Instance is a member of the cluster but has a problem that prevents it from rejoining
	CandidateDiagStatusBroken CandidateDiagStatus = "BROKEN"

	// CandidateDiagStatusUnsuitable UNSUITABLE Instance is not yet a member of the cluster and can't join it
	CandidateDiagStatusUnsuitable CandidateDiagStatus = "UNSUITABLE"

	CandidateDiagStatusUnreachable CandidateDiagStatus = "UNREACHABLE"
)

type ClusterStatusType string

const (
	ClusterStatusOnline       ClusterStatusType = "ONLINE"
	ClusterStatusOffline      ClusterStatusType = "OFFLINE"
	ClusterStatusInitializing ClusterStatusType = "INITIALIZING"
	ClusterStatusPending      ClusterStatusType = "PENDING"
	ClusterStatusFinalizing   ClusterStatusType = "FINALIZING"
	ClusterStatusFailed       ClusterStatusType = "FAILED"
)

type ClusterDiagStatusType string

const (
	// ClusterDiagStatusOnline - All members are reachable or part of the quorum
	// - Reachable members form a quorum between themselves
	// - There are no unreachable members that are not in the quorum
	// - All members are ONLINE
	ClusterDiagStatusOnline ClusterDiagStatusType = "ONLINE"

	// ClusterDiagStatusOnlinePartial - All members are reachable or part of the quorum
	// - Some reachable members form a quorum between themselves
	// - There may be members outside the quorum in any state, but they must not form a quorum
	// Note that there may be members that think are ONLINE, but minority in a view with UNREACHABLE members
	ClusterDiagStatusOnlinePartial ClusterDiagStatusType = "ONLINE_PARTIAL"

	// ClusterDiagStatusOffline - All members are reachable
	// - All cluster members are OFFLINE/ERROR (or being deleted)
	// - GTID set of all members are consistent
	// We're sure that the cluster is completely down with no quorum hiding somewhere
	// The cluster can be safely rebooted
	ClusterDiagStatusOffline ClusterDiagStatusType = "OFFLINE"

	// ClusterDiagStatusNoQuorum - All members are reachable
	// - All cluster members are either OFFLINE/ERROR or ONLINE but with no quorum
	// A split-brain with no-quorum still falls in this category
	// The cluster can be safely restored
	ClusterDiagStatusNoQuorum ClusterDiagStatusType = "NO_QUORUM"

	// ClusterDiagStatusSplitBrain - Some but not all members are unreachable
	// - There are multiple ONLINE/RECOVERING members forming a quorum, but with >1
	// different views
	// If some members are not reachable, they could either be forming more errant
	// groups or be unavailable, but that doesn't make much dfifference.
	ClusterDiagStatusSplitBrain ClusterDiagStatusType = "SPLIT_BRAIN"

	// ClusterDiagStatusOnlineUncertain - Some members are unreachable
	// - Reachable members form a quorum between themselves
	// - There are unreachable members that are not in the quorum and have unknown state
	// Because there are members with unknown state, the possibility that there's a
	// split-brain exists.
	ClusterDiagStatusOnlineUncertain ClusterDiagStatusType = "ONLINE_UNCERTAIN"

	// ClusterDiagStatusOfflineUncertain - OFFLINE with unreachable members
	ClusterDiagStatusOfflineUncertain ClusterDiagStatusType = "OFFLINE_UNCERTAIN"

	// ClusterDiagStatusNoQuorumUncertain - NoQuorum with unreachable members
	ClusterDiagStatusNoQuorumUncertain ClusterDiagStatusType = "NO_QUORUM_UNCERTAIN"

	// ClusterDiagStatusSplitBrainUncertain - SplitBrain with unreachable members
	ClusterDiagStatusSplitBrainUncertain ClusterDiagStatusType = "SPLIT_BRAIN_UNCERTAIN"

	// ClusterDiagStatusUnknown - No reachable/connectable members
	// We have no idea about the state of the cluster, so nothing can be done about it (even if we wanted)
	ClusterDiagStatusUnknown ClusterDiagStatusType = "UNKNOWN"

	// ClusterDiagStatusInitializing - Cluster is not marked as initialized in Kubernetes
	// The cluster hasn't been created/initialized yet, so we can safely create it
	ClusterDiagStatusInitializing ClusterDiagStatusType = "INITIALIZING"

	// ClusterDiagStatusFinalizing - Cluster object is marked as being deleted
	ClusterDiagStatusFinalizing ClusterDiagStatusType = "FINALIZING"

	// ClusterDiagStatusInvalid - A (currently) undiagnosable and unrecoverable mess that doesn't fit any other state
	ClusterDiagStatusInvalid ClusterDiagStatusType = "INVALID"

	ClusterDiagStatusPending ClusterDiagStatusType = "PENDING"

	ClusterDiagStatusFailed ClusterDiagStatusType = "FAILED"
)

type ConditionStatus string

// These are valid condition statuses. "ConditionTrue" means a resource is in the condition.
// "ConditionFalse" means a resource is not in the condition. "ConditionUnknown" means operator
// can't decide if a resource is in the condition or not. In the future
const (
	ConditionTrue  ConditionStatus = "True"
	ConditionFalse ConditionStatus = "False"
)

// GreatDBPaxosConditionType service state type of GreatDBPaxos
type GreatDBPaxosConditionType string

const (

	// GreatDBPaxosPending The Pending phase generates relevant configurations, such as initializing configmap, secret, service
	GreatDBPaxosPending GreatDBPaxosConditionType = "Pending"

	// GreatDBPaxosDeployDB Deploy db cluster in this phase
	GreatDBPaxosDeployDB GreatDBPaxosConditionType = "DeployDB"

	// GreatDBPaxosBootCluster Boot Cluster
	GreatDBPaxosBootCluster GreatDBPaxosConditionType = "BootCluster"

	// GreatDBPaxosInitUser Init User
	GreatDBPaxosInitUser GreatDBPaxosConditionType = "InitUser"

	// GreatDBPaxosSucceeded Successful deployment of DBscale cluster in this phase
	GreatDBPaxosSucceeded GreatDBPaxosConditionType = "Succeeded"

	// GreatDBPaxosReady The cluster is ready at this stage
	GreatDBPaxosReady GreatDBPaxosConditionType = "Ready"

	// GreatDBPaxosTerminating GreatDBPaxosDeleting The cluster is Deleting at this stage
	GreatDBPaxosTerminating GreatDBPaxosConditionType = "Terminating"

	GreatDBPaxosRepair GreatDBPaxosConditionType = "Repair"

	GreatDBPaxosPause GreatDBPaxosConditionType = "Pause"
	// GreatDBPaxosRestart Restart
	GreatDBPaxosRestart GreatDBPaxosConditionType = "Restart"
	// GreatDBPaxosUpgrade Upgrade
	GreatDBPaxosUpgrade GreatDBPaxosConditionType = "Upgrade"

	// GreatDBPaxosScaleOut Greatdb is in the scale out phase
	GreatDBPaxosScaleOut GreatDBPaxosConditionType = "ScaleOut"

	// GreatDBPaxosScaleIn Greatdb is in the scale in phase
	GreatDBPaxosScaleIn GreatDBPaxosConditionType = "ScaleIn"

	// GreatDBPaxosScaleFailed Greatdb failed to expand and shrink instances
	GreatDBPaxosScaleFailed GreatDBPaxosConditionType = "ScaleFailed"

	// GreatDBPaxosFailover Failover in progress
	GreatDBPaxosFailover GreatDBPaxosConditionType = "Failover"

	// Failover succeeded
	GreatDBPaxosFailoverSucceeded GreatDBPaxosConditionType = "FailoverSucceeded"

	// Failover failed
	GreatDBPaxosFailoverFailed GreatDBPaxosConditionType = "FailoverFailed"

	// This stage indicates cluster deployment failure
	GreatDBPaxosFailed GreatDBPaxosConditionType = "Failed"

	// This stage indicates that the correct status of the cluster cannot be obtained due to exceptions
	GreatDBPaxosUnknown GreatDBPaxosConditionType = "Unknown"
)

func (g GreatDBPaxosConditionType) Stage() int {
	switch g {
	case GreatDBPaxosPending:
		return 0
	case GreatDBPaxosDeployDB:
		return 1
	case GreatDBPaxosBootCluster:
		return 2
	case GreatDBPaxosInitUser:
		return 3
	case GreatDBPaxosSucceeded:
		return 4
	case GreatDBPaxosReady:
		return 5
	}
	return 6
}

// Status of the greatdb instance
type MemberConditionType string

const (
	MemberStatusOnline MemberConditionType = "ONLINE"

	MemberStatusRecovering MemberConditionType = "RECOVERING"
	MemberStatusError      MemberConditionType = "ERROR"
	MemberStatusOffline    MemberConditionType = "OFFLINE"

	// Unmanaged - Instance of an unmanaged replication group. Probably was already member but got removed
	MemberStatusUnmanaged MemberConditionType = "UNMANAGED"

	// Unknown - Uncertain because we can't connect or query it
	MemberStatusUnknown MemberConditionType = "UNKNOWN"

	// MemberStatusPending The instance is still starting
	MemberStatusPending MemberConditionType = "Pending"

	// MemberStatusFree The member has not yet joined the cluster
	MemberStatusFree MemberConditionType = "Free"

	// MemberStatusPause  Member Pause
	MemberStatusPause MemberConditionType = "Pause"

	// MemberStatusRestart Member restart
	MemberStatusRestart MemberConditionType = "Restart"

	// MemberStatusFailure  The member was changed to a failed state due to a long-term error
	MemberStatusFailure MemberConditionType = "Failure"
)

func (state MemberConditionType) ScaleInPriority() int {

	priority := 0
	switch state {
	case MemberStatusOnline, MemberStatusRecovering:
		priority = 0
	case MemberStatusFree, MemberStatusPending:
		priority = 1
	case MemberStatusPause, MemberStatusUnknown, MemberStatusRestart:
		priority = 2
	case MemberStatusUnmanaged, MemberStatusOffline, MemberStatusError:
		priority = 3
	case MemberStatusFailure:
		priority = 4

	}

	return priority
}

func (state MemberConditionType) Parse() MemberConditionType {

	return state
}

type MemberRoleType string

const (
	MemberRolePrimary   MemberRoleType = "PRIMARY"
	MemberRoleSecondary MemberRoleType = "SECONDARY"
	MemberRoleUnknown   MemberRoleType = "Unknown"
)

func (role MemberRoleType) Parse() MemberRoleType {

	return MemberRoleType(strings.ToUpper(string(role)))

}

type MemberCreateType string

const (
	// InitCreateMember Instance created during component clustering
	InitCreateMember MemberCreateType = "init"
	// ScaleCreateMember Instance created during expansion
	ScaleCreateMember MemberCreateType = "scaleOut"

	// FailOverCreateMember Instance created during failover
	FailOverCreateMember MemberCreateType = "failover"
)

type ScaleInStrategyType string

const (
	// Reduce in reverse order based on index
	ScaleInStrategyIndex ScaleInStrategyType = "index"

	// Prioritize scaling down faulty nodes and then reduce them in reverse order based on index
	ScaleInStrategyFault ScaleInStrategyType = "fault"

	// Shrink the specified instance
	ScaleInStrategyDefine ScaleInStrategyType = "define"
)

type ScaleOutSourceType string

const (
	ScaleOutSourceBackup ScaleOutSourceType = "backup"
	ScaleOutSourceClone  ScaleOutSourceType = "clone"
)

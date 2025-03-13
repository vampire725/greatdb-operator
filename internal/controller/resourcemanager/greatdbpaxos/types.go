package greatdbpaxos

// sql
const (
	// QueryClusterMemberStatus Query cluster status
	QueryClusterMemberStatus = "select CHANNEL_NAME,MEMBER_ID,MEMBER_HOST,MEMBER_PORT,MEMBER_STATE,MEMBER_ROLE,MEMBER_VERSION  from performance_schema.replication_group_members;"

	QueryMemberInfo = `SELECT m.member_id, m.member_host, m.member_port, m.member_role, m.member_state, s.view_id, m.member_version,
									(SELECT count(*) FROM performance_schema.replication_group_members) as member_count,
									(SELECT count(*) FROM performance_schema.replication_group_members WHERE member_state <> 'UNREACHABLE') as reachable_member_count
							FROM performance_schema.replication_group_members m
								JOIN performance_schema.replication_group_member_stats s
								ON m.member_id = s.member_id
							WHERE m.member_id = @@server_uuid`
	QueryMembers = `SELECT m.member_id, m.member_role, m.member_state,
						s.view_id, concat(m.member_host, ':', m.member_port) as address, m.member_version
					FROM performance_schema.replication_group_members m
						JOIN performance_schema.replication_group_member_stats s
						ON m.member_id = s.member_id`
)

const (
	// greatdb
	// The name of the greatdb mount configuration
	greatdbConfigMountPath string = "/etc/greatdb/"
	greatdbDataMountPath   string = "/greatdb/mysql/"
	greatdbBackupMountPath string = "/backup/"
	dashboardDataMountPath string = "/data"

	DashboardContainerName    = "dashboard"
	GreatDBContainerName      = "greatdb"
	GreatDBAgentContainerName = "greatdb-agent"
	BackupServerPort          = 19999
)

// pvc
const (
	StorageVerticalShrinkagegprohibit  = "Storage is prohibited from shrinking and configuration is rolled back"
	StorageVerticalExpansionNotSupport = "Storage does not support dynamic expansion"
)

type GtidExecuted struct {
	GtidExecuted string `json:"@@gtid_executed,omitempty"`
}

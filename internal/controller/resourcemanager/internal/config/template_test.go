package config

import (
	"fmt"
	"testing"
)

type DbscaleConfigs struct {
	ZkHost             string
	ClusterUser        string
	CusterUserPassword string
}

const dbscaleConfigTemplate = `
[main]
zookeeper-host = {{ .ZkHost }}
zookeeper-password = ZGJzY2FsZQ==

admin-user = {{ .ClusterUser }}
cluster-user = {{ .ClusterUser  }}
supreme-admin-user = {{ .ClusterUser  }}
dbscale-internal-user = {{ .ClusterUser  }}
normal-admin-user = {{ .ClusterUser  }}

admin-password = {{ .CusterUserPassword }}
cluster-password = {{ .CusterUserPassword }}

log-file = /greatdb/dbscale/logs/dbscale.log
pid-file = /greatdb/dbscale/pid/dbscale.pid
zk-log-file =  /greatdb/dbscale/logs/zookeeper.log
`

func TestTemplate(t *testing.T) {
	tmpl := NewconfigTemplate()
	err := tmpl.LoadStringTemplate("dbscale", dbscaleConfigTemplate)
	if err != nil {
		fmt.Println(err)
	}
	config := DbscaleConfigs{ClusterUser: "1232", CusterUserPassword: "1232", ZkHost: "1232"}

	data1, err := tmpl.ExecToString(config)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(data1)

}

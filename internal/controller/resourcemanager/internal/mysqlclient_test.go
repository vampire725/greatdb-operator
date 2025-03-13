package internal

import (
	"fmt"
	"testing"
)

func TestMysqlClient(t *testing.T) {

	client := NewDBClient()
	user := "root"
	host := "172.16.50.50"
	pass := "root123"
	port := 3306
	dbname := ""

	err := client.Connect(user, pass, host, port, dbname)
	if err != nil {
		return
	}

	dbVariables := []DBVariable{}

	dbVariables = append(dbVariables, DBVariable{key: "wait_timeout", value: "31535000"})
	dbVariables = append(dbVariables, DBVariable{key: "net_write_timeout", value: "10002"})
	dbVariables = append(dbVariables, DBVariable{key: "slow_query_log", value: "on"})

	err = client.UpdateDBVariables(dbVariables)
	if err != nil {
		fmt.Println(err.Error())
	}

}

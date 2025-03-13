/**
 * Copyright (c) 2018 Dell Inc., or its subsidiaries. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 */

package config

var (
	KubeConfig string
	MasterHost string
	LogLever   int
)

type apiVersion struct {
	PDB string
}

// k8s server info
var (
	ApiVersion apiVersion

	// k8s version
	ServerVersion string
)

// operator
var (
	ManagerBy            string = "greatdb-operator"
	ServiceType          string = "GreatDBPaxos"  //
	DefaultClusterDomain string = "cluster.local" //
)

package v1

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	greatdbv1 "greatdb.com/greatdb-operator/api/v1"
)

/*
 * @Author: Gpp
 * @File:   common.go
 * @Date:   2025/3/11 下午8:45
 */

func ValidateClusterName(ctx context.Context, v client.Client, fieldPath *field.Path, namespace string, clusterName string) *field.Error {
	if clusterName == "" {
		return field.Required(fieldPath, "cluster name is required")
	}

	cluster := &greatdbv1.GreatDBPaxos{}
	if err := v.Get(ctx, types.NamespacedName{Namespace: namespace, Name: clusterName}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			return field.InternalError(fieldPath, err)
		}
	}

	return nil
}

func ValidateInstanceName(ctx context.Context, v client.Client, fieldPath *field.Path, namespace, clusterName, instanceName string) *field.Error {
	if clusterName != "" && instanceName != "" {
		cluster := &greatdbv1.GreatDBPaxos{}
		if err := v.Get(ctx, types.NamespacedName{Namespace: namespace, Name: clusterName}, cluster); err != nil {
			return nil
		}

		exist := false

		for _, mem := range cluster.Status.Member {
			if mem.Name == instanceName {
				exist = true
				break
			}
		}

		if !exist {
			return field.Invalid(fieldPath, fmt.Sprintf("instance name %s not exist", instanceName), "")
		}
	}

	return nil
}

package configmap

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	greatdbv1 "greatdb.com/greatdb-operator/api/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

type Manager struct {
	Client   client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
}

func (config *Manager) Sync(ctx context.Context, Cluster *greatdbv1.GreatDBPaxos) error {

	greatdb := NewGreatdbConfigManager(config.Client, config.Recorder, config.Scheme)
	// sync configmap of greatdb
	if err := greatdb.Sync(ctx, Cluster); err != nil {
		config.Recorder.Eventf(Cluster, corev1.EventTypeNormal, SyncGreatDbConfigmapFailedReason, err.Error())
		return err
	}

	return nil

}

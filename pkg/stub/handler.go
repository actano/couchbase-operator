package stub

import (
	"context"

	"github.com/actano/couchbase-operator/pkg/apis/operators/v1alpha1"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	couchbase "github.com/couchbase/gocbmgr"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func NewHandler() sdk.Handler {
	return &Handler{}
}

type Handler struct {
	// Fill me
}

func (h *Handler) Handle(ctx context.Context, event sdk.Event) error {
	switch o := event.Object.(type) {
	case *v1alpha1.Couchbase:
		pod := newCouchbasePod(o)
		err := sdk.Create(pod)
		if err != nil && !errors.IsAlreadyExists(err) {
			logrus.Errorf("Failed to create couchbase pod : %v", err)
			return err
		}
		// Ensure the deployment size is the same as the spec
		err = sdk.Get(pod)
		if err != nil {
			logrus.Errorf("failed to get deployment: %v", err)
			return err
		}

		containerIsReady := pod.Status.ContainerStatuses[0].Ready

		if !containerIsReady {
			logrus.Infof("Not ready to initialize")
			return nil
		}

		podIp := pod.Status.PodIP
		logrus.Infof("Couchbase PodIp: %s", podIp)



		couchbaseClient := couchbase.New("admin", "password")
		couchbaseClient.SetEndpoints([]string{"http://" + podIp + ":8091"})


		_, err = couchbaseClient.ClusterInfo()
		if err != nil {

			err := initalizeCluster(couchbaseClient)
			if err != nil {
				logrus.Errorf("failed to initialize cluster: %v", err)
				return err
			}
		}
	}
	return nil
}

func initalizeCluster(couchbaseClient *couchbase.Couchbase) error {
	poolDefaults := couchbase.PoolsDefaults{ClusterName: "default", IndexMemoryQuota: 256, DataMemoryQuota: 256, SearchMemoryQuota: 256}
	return couchbaseClient.ClusterInitialize("admin", "password", &poolDefaults, 8091, []couchbase.ServiceName{couchbase.DataService}, "" )
}

// newbusyBoxPod demonstrates how to create a busybox pod
func newCouchbasePod(cr *v1alpha1.Couchbase) *corev1.Pod {
	labels := labelsForCouchbase(cr.Name)
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "couchbase",
			Namespace: cr.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(cr, schema.GroupVersionKind{
					Group:   v1alpha1.SchemeGroupVersion.Group,
					Version: v1alpha1.SchemeGroupVersion.Version,
					Kind:    "Couchbase",
				}),
			},
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "couchbase",
					Image:   cr.Spec.Image,
					ReadinessProbe: &corev1.Probe{
						InitialDelaySeconds: 20,
						Handler: corev1.Handler{
							HTTPGet: &corev1.HTTPGetAction{
								Scheme: "HTTP",
								Port:   intstr.FromInt(8091),
							},
						},
					},
				},
			},
		},
	}
}

// labelsForCouchbase returns the labels for selecting the resources
// belonging to the given memcached CR name.
func labelsForCouchbase(name string) map[string]string {
	return map[string]string{"app": "couchbase", "couchbase_cr": name}
}

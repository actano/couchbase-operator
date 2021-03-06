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
	"fmt"
	"strings"
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
		service := newCouchbaseService(o)
		err := sdk.Create(service)
		if err != nil && !errors.IsAlreadyExists(err) {
			logrus.Errorf("Failed to create couchbase service : %v", err)
			return err
		}

		size := o.Spec.Size

		var pods []*corev1.Pod

		for i := int32(1); i <= size; i++ {
			pod, err := createPod(o, fmt.Sprintf("%03d", i))
			if err != nil {
				switch err.(type) {
				case *podNotReadyError:
					continue
				default:
					logrus.Errorf("failed to create pod: %v", err)
					return err
				}
			}
			pods = append(pods, pod)
		}

		if int32(len(pods)) != size {
			return nil
		}

		podIp := pods[0].Status.PodIP
		couchbaseClient := couchbase.New("admin", "password")
		couchbaseClient.SetEndpoints([]string{"http://" + podIp + ":8091"})


		clusterInfo, err := couchbaseClient.ClusterInfo()
		if err != nil {

			// initialize node...
			hostname := podDNSName(*pods[0])
			datapath := "/opt/couchbase/var/lib/couchbase/data"
			err = couchbaseClient.NodeInitialize(hostname, datapath, datapath, []string{})
			if err != nil {
				logrus.Errorf("failed to initialize node: %v", err)
				return err
			}

			err := initalizeCluster(couchbaseClient)
			if err != nil {
				logrus.Errorf("failed to initialize cluster: %v", err)
				return err
			}
			logrus.Info("Initialized Cluster")
			return nil
		}
		nodes := clusterInfo.Nodes

		allNodesInCluster := true
		for _, pod := range pods {
			hostname := podDNSName(*pod)
			isInCluster := false
			for _, node := range nodes {
				nodeHostename := node.HostName
				if strings.Contains(nodeHostename, hostname) {
					isInCluster = true
					break
				}
			}
			if !isInCluster {
				allNodesInCluster = false
				err := joinCluster(couchbaseClient, pod)
				if err != nil {
					logrus.Errorf("Error Joining the cluster: %v", err)
					continue
				}
				logrus.Infof("Successfully added node: %v", hostname)
			}
		}

		if allNodesInCluster {
			progress, err := couchbaseClient.Rebalance([]string{})
			if err != nil {
				logrus.Errorf("Error Rebalancing Cluster: %v", err)
				return err
			}

			err = progress.Wait()
			if err != nil {
				logrus.Errorf("Error Waiting for Rebalancing Cluster: %v", err)
				return err
			}
			logrus.Infof("Successfully rebalanced the cluster")
		}
	}

	return nil
}

type podNotReadyError struct {
	message    string
}

func newPodNotReadyError(message string) *podNotReadyError {
	return &podNotReadyError{message: message}
}

func (e *podNotReadyError) Error() string {
	return e.message
}

func createPod(cr *v1alpha1.Couchbase, id string) (*corev1.Pod, error) {
	pod := newCouchbasePod(cr, id)
	err := sdk.Create(pod)
	if err != nil && !errors.IsAlreadyExists(err) {
		logrus.Errorf("Failed to create couchbase pod : %v", err)
		return nil, err
	}
	// Ensure the deployment size is the same as the spec
	err = sdk.Get(pod)
	if err != nil {
		logrus.Errorf("failed to get deployment: %v", err)
		return nil, err
	}

	containerIsReady := len(pod.Status.ContainerStatuses) != 0 && pod.Status.ContainerStatuses[0].Ready

	if !containerIsReady {
		return nil, newPodNotReadyError("Pod " + id + " not ready")
	}

	return pod, nil
}

func initalizeCluster(couchbaseClient *couchbase.Couchbase) error {
	poolDefaults := couchbase.PoolsDefaults{ClusterName: "default", IndexMemoryQuota: 256, DataMemoryQuota: 256, SearchMemoryQuota: 256}
	return couchbaseClient.ClusterInitialize("admin", "password", &poolDefaults, 8091, []couchbase.ServiceName{couchbase.DataService}, "" )
}

func joinCluster(couchbaseClient *couchbase.Couchbase, pod *corev1.Pod) error {
	hostname := podDNSName(*pod)
	return couchbaseClient.AddNode(hostname, "admin", "password",[]couchbase.ServiceName{couchbase.DataService})
}

func newCouchbaseService(cr *v1alpha1.Couchbase) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cr.Namespace,
			Name:      "couchbase",

		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 8091,
				},
			},
			Selector: labelsForCouchbase(cr.Name),
		},
	}
}

// newbusyBoxPod demonstrates how to create a busybox pod
func newCouchbasePod(cr *v1alpha1.Couchbase, id string) *corev1.Pod {
	labels := labelsForCouchbase(cr.Name)
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "couchbase" + id,
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

// podDNSName constructs the dns name on which a pod can be addressed
func podDNSName(p corev1.Pod) string {
	podIP := strings.Replace(p.Status.PodIP, ".", "-", -1)
	return fmt.Sprintf("%s.%s.pod", podIP, p.Namespace)
}

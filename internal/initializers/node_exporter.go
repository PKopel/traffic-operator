package initializers

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	NodeExporterServiceName   = "traffic-operator-node-exporter-service"
	NodeExporterDaemonSetYaml = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
  labels:
    app.kubernetes.io/component: traffic-operator-exporter
    app.kubernetes.io/name: traffic-operator-node-exporter
  name: traffic-operator-node-exporter
spec:
  selector:
    matchLabels:
      app.kubernetes.io/component: traffic-operator-exporter
      app.kubernetes.io/name: traffic-operator-node-exporter
  template:
    metadata:
      labels:
        app.kubernetes.io/component: traffic-operator-exporter
        app.kubernetes.io/name: traffic-operator-node-exporter
    spec:
      containers:
        - args:
            - --path.sysfs=/host/sys
            - --path.rootfs=/host/root
            - --no-collector.wifi
            - --no-collector.hwmon
            - --collector.filesystem.ignored-mount-points=^/(dev|proc|sys|var/lib/docker/.+|var/lib/kubelet/pods/.+)($|/)
            - --collector.netclass.ignored-devices=^(veth.*)$
          name: traffic-operator-node-exporter
          image: prom/node-exporter
          ports:
            - containerPort: 9100
              protocol: TCP
          resources:
            limits:
              cpu: 250m
              memory: 180Mi
            requests:
              cpu: 102m
              memory: 180Mi
          volumeMounts:
            - mountPath: /host/sys
              mountPropagation: HostToContainer
              name: sys
              readOnly: true
            - mountPath: /host/root
              mountPropagation: HostToContainer
              name: root
              readOnly: true
      volumes:
        - hostPath:
            path: /sys
          name: sys
        - hostPath:
            path: /
          name: root
`
)

// Namespace in which Node Exporter is to be deployed
var namespace = "traffic-operator-system"

func GetNamespace() string {
	return namespace
}

// RBAC for creating DaemonSet
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=create
// RBAC for Pods
// +kubebuilder:rbac:groups="",resources=pods,verbs=list;watch

// InitializeNodeExporter creates Node Exporter's DaemonSet
func InitializeNodeExporter(mgr ctrl.Manager, ns string) error {
	ctx := context.Background()
	logger := ctrl.Log.WithName("setup")

	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDecoder()
	daemonSet := &appsv1.DaemonSet{}

	err := runtime.DecodeInto(decoder, []byte(NodeExporterDaemonSetYaml), daemonSet)
	if err != nil {
		return err
	}

	// uncached client
	c, err := client.New(mgr.GetConfig(), client.Options{})
	if err != nil {
		return err
	}

	pods := &corev1.PodList{}
	listOptions := client.MatchingLabels{"app.kubernetes.io/name": "traffic-operator"}

	logger.Info("listing traffic-operator pods")
	if err := c.List(ctx, pods, listOptions); err != nil {
		logger.Error(err, "error while listing traffic-operator pods")
	}

	if len(pods.Items) > 0 {
		logger.Info("found traffic-operator pods", "#pods", len(pods.Items), "namespace", pods.Items[0].Namespace)
		namespace = pods.Items[0].Namespace
	} else {
		logger.Info("no traffic-operator pods found")
		namespace = ns
	}

	daemonSet.Namespace = namespace

	if err := c.Create(ctx, daemonSet); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

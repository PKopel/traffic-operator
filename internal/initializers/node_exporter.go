package initializers

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const NodeExporterDaemonSetYaml = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
  labels:
    app.kubernetes.io/component: traffic-operator-exporter
    app.kubernetes.io/name: traffic-operator-node-exporter
  name: traffic-operator-node-exporter
  namespace: system
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

func InitializeNodeExporter(client client.Client) error {
	ctx := context.Background()

	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDecoder()
	daemonSet := &appsv1.DaemonSet{}

	err := runtime.DecodeInto(decoder, []byte(NodeExporterDaemonSetYaml), daemonSet)
	if err != nil {
		return err
	}

	if err := client.Create(ctx, daemonSet); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

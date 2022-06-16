# Traffic Operator

## Deployment

There are two ways to deploy the Traffic Operator:

1. With OLM (use `make olm-install` to install OLM in case it's not installed):

    ```sh
    make olm-deploy
    ```

2. Without OLM:

    ```sh
    make install deploy
    ```

Operator can be installed in any namespace, the default is `traffic-operator-system`.
Repo also contains mock deployment [traffic_generator.yaml](config/samples/traffic_generator.yaml).

Docker images used by Traffic Operator:

- `pkopel/traffic-operator-catalog` - image for OLM's `CatalogSource` CR
- `pkopel/traffic-operator-bundle` - CRDs and other resources for OLM
- `pkopel/traffic-operator` - controller image

## Overview

Traffic Operator monitors network usage on worker nodes and marks them with labels based
on threshold configured in `TrafficWatch` CR. Network usage metrics are provided by
[Node Exporter](https://prometheus.io/docs/guides/node-exporter/) `DaemonSet` deployed by
the operator on [startup](internal/initializers/node_exporter.go).

### Custom Resource

Traffic Operator uses one simple Custom Resource named [`TrafficWatch`](config/samples/traffic_v1alpha1_trafficwatch.yaml):

```yaml
apiVersion: traffic.example.com/v1alpha1
kind: TrafficWatch
metadata:
  name: trafficwatch-sample
spec:
  maxBandwidthPercent: "60"
  deployment:selector:
      matchLabels:
        app.kubernetes.io/component: trafficwatch-sample
        app.kubernetes.io/name: trafficwatch-sample
    replicas: 2
    template:
      metadata:
        labels:
          app.kubernetes.io/component: trafficwatch-sample
          app.kubernetes.io/name: trafficwatch-sample
      spec:
        containers:
          - name: example-app
            image: example-app/image
            resources:
              limits:
                cpu: 500m
                memory: 128Mi
              requests:
                cpu: 10m
                memory: 64Mi
            command: ["app"]
```

Important: `spec.maxBandwidthPercent` is a string, not a number, to allow for use of fractions (like `1.5e-03`).

Traffic Operator will create a deployment based on configuration provided in `TrafficWatch`
and configure `nodeAffinity` so that pods will be created only on 'fit' nodes. Operator will mark
nodes with labels specific for each `TrafficWatch` resource.

Reconciled `TrafficWatch` contains information about current network traffic on each worker node:

```yaml
apiVersion: traffic.example.com/v1alpha1
kind: TrafficWatch
metadata:
  name: trafficwatch-sample
  namespace: default
spec:
  maxBandwidthPercent: "60"
  deployment:selector:
      matchLabels:
        app.kubernetes.io/component: trafficwatch-sample
        app.kubernetes.io/name: trafficwatch-sample
    replicas: 2
    template:
      metadata:
        labels:
          app.kubernetes.io/component: trafficwatch-sample
          app.kubernetes.io/name: trafficwatch-sample
      spec:
        containers:
          - name: example-app
            image: example-app/image
            resources:
              limits:
                cpu: 500m
                memory: 128Mi
              requests:
                cpu: 10m
                memory: 64Mi
            command: ["app"]
status:
  nodes:
    kind-worker:
      currentBandwidthPercent: "1.06976e-04"
      currentTransmitTotal: 281814
      time: 1654276365
      unfit: false
    kind-worker2:
      currentBandwidthPercent: "9.784e-05"
      currentTransmitTotal: 283700
      time: 1654276366
      unfit: false
```

### Reconciliation loop

[Reconciliation loop](internal/controllers/trafficwatch_controller.go) consists of following steps:

1. Finding Node Exporter endpoints on each worker node

2. Polling each endpoint to get current network usage information

3. Computing average bandwidth used on each worker node from information saved in `TrafficWatch` and metrics provided by Node Exporter

4. Updating labels on each node

5. Creating/updating `Deployment` defined in `TrafficWatch`

6. Saving current statistics in `TrafficWatch`

Reconciliation loop runs every 10s for each `TrafficWatch` CR if everything is alright, in case of errors reconciler waits 30s before next attempt.
To limit number of reconciliation loop runs, controller uses a filter discarding update events that don't change `spec` field of the `TrafficWatch` CR.

### Repo contents

- [bundle](bundle) - OLM bundle resources
- [config](config) - yaml files
- [internal](internal) - controller code
- [api](api/v1alpha1/) - api definitions

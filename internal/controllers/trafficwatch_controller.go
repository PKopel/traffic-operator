/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/PKopel/traffic-operator/api/v1alpha1"
	trafficv1alpha1 "github.com/PKopel/traffic-operator/api/v1alpha1"
	"github.com/PKopel/traffic-operator/internal/utils"
	corev1 "k8s.io/api/core/v1"
)

const (
	reconcileTimeout = 1 * time.Minute

	shortWait = 10 * time.Second
	longWait  = 30 * time.Minute

	nodeNetworkSpeed    = "node_network_speed_bytes"
	nodeNetworkTransmit = "node_network_transmit_bytes_total"
)

// TrafficWatchReconciler reconciles a TrafficWatch object
type TrafficWatchReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=traffic.example.com,resources=trafficwatches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=traffic.example.com,resources=trafficwatches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=traffic.example.com,resources=trafficwatches/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the TrafficWatch object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *TrafficWatchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	ctx, cancel := context.WithTimeout(ctx, reconcileTimeout)
	defer cancel()

	logger := log.FromContext(ctx)
	logger.Info("Reconciling...")

	tw := &v1alpha1.TrafficWatch{}
	if err := r.Get(ctx, req.NamespacedName, tw); err != nil {
		logger.Info("Get TrafficWatch error")
		return ctrl.Result{}, err
	}

	pods := &corev1.PodList{}

	listOptions := client.MatchingLabels{"app.kubernetes.io/component": "traffic-operator-exporter"}

	if err := r.List(ctx, pods, listOptions); err != nil {
		logger.Info("List Nodes error")
		return ctrl.Result{RequeueAfter: longWait}, err
	}

	for i, pod := range pods.Items {
		addr := pod.Status.PodIP
		url := fmt.Sprintf("http://%s:9100/metrics", addr)

		time := time.Now().Unix()

		resp, err := http.Get(url)
		if err != nil {
			logger.Info("Metric request error")
			return ctrl.Result{RequeueAfter: longWait}, err
		}

		metrics, err := utils.ParseAll(resp.Body)
		if err != nil {
			logger.Info("Metric parsing error")
			return ctrl.Result{RequeueAfter: longWait}, err
		}

		speedMetrics := utils.Filter(metrics, func(m utils.Metric) bool {
			return m.Name == nodeNetworkSpeed
		})

		transmitMetrics := utils.Filter(metrics, func(m utils.Metric) bool {
			return m.Name == nodeNetworkTransmit
		})

		var totalSpeed, totalTransmit float64

		for _, sm := range speedMetrics {
			tm := utils.First(transmitMetrics, func(m utils.Metric) bool {
				return m.Labels["device"] == sm.Labels["device"]
			})
			if tm == nil {
				continue
			}

			smv, err := strconv.ParseFloat(sm.Value, 64)
			if err != nil {
				logger.Info("Could not parse smv float")
				continue
			}
			tmv, err := strconv.ParseFloat(tm.Value, 64)
			if err != nil {
				logger.Info("Could not parse tmv float")
				continue
			}
			totalSpeed += smv
			totalTransmit += tmv
		}

		if len(tw.Status.Nodes) == len(pods.Items) {
			prevTime := tw.Status.Nodes[i].Time
			prevTransmit := float64(tw.Status.Nodes[i].CurrentTransmitTotal)
			bwPercent := int64(math.Round((totalTransmit - prevTransmit) * 100 / (float64(time-prevTime) * totalSpeed)))

			tw.Status.Nodes[i].CurrentTransmitTotal = int64(totalTransmit)
			tw.Status.Nodes[i].CurrentBandwidthPercent = bwPercent
			tw.Status.Nodes[i].Time = time
			tw.Status.Nodes[i].Unfit = bwPercent > tw.Spec.MaxBandwidthPercent
		} else {
			node := trafficv1alpha1.CurrentNodeTraffic{
				CurrentTransmitTotal:    int64(totalTransmit),
				CurrentBandwidthPercent: 0,
				Time:                    time,
				Unfit:                   false,
			}

			tw.Status.Nodes = append(tw.Status.Nodes, node)
		}
	}

	if err := r.Status().Update(ctx, tw); err != nil {
		logger.Info("Update TrafficWatch error")
		return ctrl.Result{RequeueAfter: longWait}, err
	}

	return ctrl.Result{RequeueAfter: shortWait}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TrafficWatchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&trafficv1alpha1.TrafficWatch{}).
		Complete(r)
}

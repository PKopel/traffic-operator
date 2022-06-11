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
	"net/http"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/PKopel/traffic-operator/api/v1alpha1"
	trafficv1alpha1 "github.com/PKopel/traffic-operator/api/v1alpha1"
	"github.com/PKopel/traffic-operator/internal/initializers"
	"github.com/PKopel/traffic-operator/internal/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	reconcileTimeout = 1 * time.Minute

	shortWait = 10 * time.Second
	longWait  = 30 * time.Minute

	nodeNetworkSpeed    = "node_network_speed_bytes"
	nodeNetworkTransmit = "node_network_transmit_bytes_total"

	trafficWatchLabel = "%s.traffic.example.com/unfit"
	metricsUrl        = "http://%s:9100/metrics"
)

// TrafficWatchReconciler reconciles a TrafficWatch object
type TrafficWatchReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// RBAC for TrafficWatch
//+kubebuilder:rbac:groups=traffic.example.com,resources=trafficwatches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=traffic.example.com,resources=trafficwatches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=traffic.example.com,resources=trafficwatches/finalizers,verbs=update

// Reconcile updates TrafficWatch with current network usage statistics from worker nodes
func (r *TrafficWatchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	ctx, cancel := context.WithTimeout(ctx, reconcileTimeout)
	defer cancel()

	logger := log.FromContext(ctx)
	logger.Info("reconciling...")

	tw := &v1alpha1.TrafficWatch{}
	if err := r.Get(ctx, req.NamespacedName, tw); err != nil {
		logger.Info("get TrafficWatch error")
		return ctrl.Result{}, err
	}

	if err := r.updateMetrics(ctx, tw); err != nil {
		logger.Info("update metrics error")
		return ctrl.Result{RequeueAfter: longWait}, err
	}

	if err := r.updateDeployment(ctx, tw); err != nil {
		logger.Info("update deployment error")
		return ctrl.Result{RequeueAfter: longWait}, err
	}

	if err := r.Status().Update(ctx, tw); err != nil {
		logger.Info("update TrafficWatch error")
		return ctrl.Result{RequeueAfter: longWait}, err
	}

	return ctrl.Result{RequeueAfter: shortWait}, nil
}

// RBAC for Endpoints
//+kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
// RBAC for Nodes
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update

// updateMetrics update TraficWatch status with current metrics
func (r *TrafficWatchReconciler) updateMetrics(ctx context.Context, tw *v1alpha1.TrafficWatch) error {

	logger := log.FromContext(ctx)
	logger.Info("updating status...")

	if len(tw.Status.Nodes) == 0 {
		tw.Status.Nodes = make(map[string]trafficv1alpha1.CurrentNodeTraffic)
	}

	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes); err != nil {
		logger.Error(err, "list Nodes error")
		return err
	}

	endpoints := &corev1.Endpoints{}
	endpointsName := types.NamespacedName{
		Namespace: initializers.GetNamespace(),
		Name:      initializers.NodeExporterServiceName,
	}

	if err := r.Get(ctx, endpointsName, endpoints); err != nil {
		logger.Error(err, "list Endpoints error")
		return err
	}

	addresses := endpoints.Subsets[0].Addresses

	for _, address := range addresses {

		time := time.Now().Unix()

		url := fmt.Sprintf(metricsUrl, address.IP)

		resp, err := http.Get(url)
		if err != nil {
			logger.Error(err, "metric request error")
			return err
		}

		metrics, err := utils.ParseAll(resp.Body)
		if err != nil {
			logger.Error(err, "metric parsing error")
			return err
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
				logger.Error(err, "could not parse smv float")
				continue
			}
			tmv, err := strconv.ParseFloat(tm.Value, 64)
			if err != nil {
				logger.Error(err, "could not parse tmv float")
				continue
			}
			totalSpeed += smv
			totalTransmit += tmv
		}

		var nt trafficv1alpha1.CurrentNodeTraffic
		node := utils.First(nodes.Items, func(n corev1.Node) bool {
			return n.Name == *address.NodeName
		})

		var twLabel string
		if tw.Spec.Label != "" {
			twLabel = fmt.Sprintf(trafficWatchLabel, tw.Spec.Label)
		} else {
			twLabel = fmt.Sprintf(trafficWatchLabel, tw.Name)
		}

		if len(tw.Status.Nodes) == len(addresses) {

			nt = tw.Status.Nodes[*address.NodeName]

			prevTime := nt.Time
			prevTransmit := float64(nt.CurrentTransmitTotal)
			bwPercent := (totalTransmit - prevTransmit) * 100 / (float64(time-prevTime) * totalSpeed)
			bwMax, err := strconv.ParseFloat(tw.Spec.MaxBandwidthPercent, 64)
			if err != nil {
				logger.Error(err, "could not parse maxBandwidthPercent float")
				continue
			}
			unfit := bwPercent > bwMax

			nt.CurrentTransmitTotal = int64(totalTransmit)
			nt.CurrentBandwidthPercent = strconv.FormatFloat(bwPercent, 'e', -1, 64)
			nt.Time = time
			nt.Unfit = unfit

			node.Labels[twLabel] = strconv.FormatBool(unfit)
		} else {
			nt = trafficv1alpha1.CurrentNodeTraffic{
				CurrentTransmitTotal:    int64(totalTransmit),
				CurrentBandwidthPercent: "0",
				Time:                    time,
				Unfit:                   false,
			}
			node.Labels[twLabel] = "false"
		}

		tw.Status.Nodes[*address.NodeName] = nt
		if err := r.Update(ctx, node); err != nil {
			logger.Error(err, "update Node error")
			return err
		}
	}
	return nil
}

// RBAC for Deploymnets
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;create;update

// updateDeployment update managed deployment
func (r *TrafficWatchReconciler) updateDeployment(ctx context.Context, tw *v1alpha1.TrafficWatch) error {

	logger := log.FromContext(ctx)
	logger.Info("updating deployment...")

	deploy := &appsv1.Deployment{}
	deploy.Name = tw.Name
	deploy.Namespace = tw.Namespace

	or := v1.NewControllerRef(tw, tw.GroupVersionKind())
	deploy.OwnerReferences = []v1.OwnerReference{*or}

	deploy.Spec = tw.Spec.Deployment

	nst := corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      fmt.Sprintf(trafficWatchLabel, tw.Spec.Label),
				Operator: corev1.NodeSelectorOpNotIn,
				Values:   []string{"true"},
			},
		},
	}

	a := deploy.Spec.Template.Spec.Affinity
	if a == nil {
		a = &corev1.Affinity{}
	}
	na := a.NodeAffinity
	if na == nil {
		na = &corev1.NodeAffinity{}
	}

	rdside := na.RequiredDuringSchedulingIgnoredDuringExecution
	if rdside == nil {
		rdside = &corev1.NodeSelector{}
		rdside.NodeSelectorTerms = []corev1.NodeSelectorTerm{nst}
	} else {
		rdside.NodeSelectorTerms = append(rdside.NodeSelectorTerms, nst)
	}

	a.NodeAffinity = na
	deploy.Spec.Template.Spec.Affinity = a

	if err := r.Update(ctx, deploy); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, deploy); err != nil {
				logger.Error(err, "error updating deployment")
				return err
			}
			logger.Error(err, "error creating deployment")
			return err
		}
	}

	return nil
}

// filterStatusUpdates blocks reconciliations caused by status updates
func filterStatusUpdates() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if oldTw, ok := e.ObjectOld.(*trafficv1alpha1.TrafficWatch); ok {
				newTw, _ := e.ObjectNew.(*trafficv1alpha1.TrafficWatch)
				// Is TrafficWatch, ignore status and metadata updates
				mbp := newTw.Spec.MaxBandwidthPercent != oldTw.Spec.MaxBandwidthPercent
				l := newTw.Spec.Label != oldTw.Spec.Label
				return mbp || l
			}
			// Is not TrafficWatch
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *TrafficWatchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&trafficv1alpha1.TrafficWatch{}).
		WithEventFilter(filterStatusUpdates()).
		Complete(r)
}

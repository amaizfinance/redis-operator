// Copyright 2019 The redis-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/cenkalti/backoff/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	k8sv1alpha1 "github.com/amaizfinance/redis-operator/api/v1alpha1"
	"github.com/amaizfinance/redis-operator/controllers/redis"
)

var (
	log = logf.Log.WithName("controller_redis")
	// used to check if the password is a simple alphanumeric string
	isAlphaNumeric = regexp.MustCompile(`^[[:alnum:]]+$`).MatchString
)

// RedisReconcile reconciles a Redis object
type RedisReconcile struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Redis object and makes changes based on the state read
// and what is in the Redis.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *RedisReconcile) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.WithValues("Namespace", request.Namespace, "Redis", request.Name)
	loggerDebug := logger.V(1).Info
	loggerDebug("Reconciling Redis")

	// Fetch the Redis instance
	fetchedRedis := new(k8sv1alpha1.Redis)
	if err := r.Client.Get(ctx, request.NamespacedName, fetchedRedis); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// work with the copy
	redisObject := fetchedRedis.DeepCopy()
	// initialize options
	options := objectGeneratorOptions{serviceType: serviceTypeAll}
	// adding some default labels on top of user-defined
	if redisObject.Labels == nil {
		redisObject.Labels = make(map[string]string)
	}
	redisObject.Labels[redisName] = redisObject.GetName()

	// read password from Secret
	if redisObject.Spec.Password.SecretKeyRef != nil {
		passwordSecret := new(corev1.Secret)
		if err := r.Client.Get(ctx, types.NamespacedName{
			Namespace: request.Namespace,
			Name:      redisObject.Spec.Password.SecretKeyRef.Name,
		}, passwordSecret); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to fetch password: %s", err)
		}

		if redisObject.Spec.Annotations == nil {
			redisObject.Spec.Annotations = make(map[string]string)
		}

		options.password = string(passwordSecret.Data[redisObject.Spec.Password.SecretKeyRef.Key])
		// Warning: since Redis is pretty fast an outside user can try up to
		// 150k passwords per second against a good box. This means that you should
		// use a very strong password otherwise it will be very easy to break.
		if len(options.password) < 8 && isAlphaNumeric(options.password) {
			logger.Info("WARNING: The password looks weak, please change it.")
		}
	}

	// create or update resources
	for i, object := range []client.Object{
		new(corev1.Service), new(corev1.Service), new(corev1.Service), // 3 distinct services ;)
		new(corev1.Secret),
		new(corev1.ConfigMap),
		new(v1.PodDisruptionBudget),
		new(appsv1.StatefulSet),
	} {
		switch object.(type) {
		case *corev1.ConfigMap, *policyv1beta1.PodDisruptionBudget, *appsv1.StatefulSet:
		// nothing special to do here
		case *corev1.Secret:
			if len(options.password) == 0 {
				continue
			}
		case *corev1.Service:
			// a bit hacky way to create three different instances of *v1.Service
			// without copy-pasting and introducing all the corresponding risks
			options.serviceType = serviceTypeAll + i
		default:
			// unknown type
			continue
		}

		if result, err := r.createOrUpdate(ctx, object, redisObject, options); err != nil {
			return ctrl.Result{}, err
		} else if result.Requeue {
			logger.Info(fmt.Sprintf("Applied %T", object))
			return result, nil
		}
	}

	// all the kubernetes resources are OK.
	// Redis failover state should be checked and reconfigured if needed.
	podList := new(corev1.PodList)
	listOpts := []client.ListOption{
		client.InNamespace(request.Namespace),
		client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(redisObject.Labels)},
	}
	if err := r.Client.List(ctx, podList, listOpts...); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list Pods: %s", err)
	}

	var addresses []redis.Address

podIter:
	// filter out pods without assigned IP addresses and not having all containers ready
	for i := range podList.Items {
		if podList.Items[i].Status.Phase != corev1.PodRunning || podList.Items[i].Status.PodIP == "" {
			continue
		}

		for _, status := range podList.Items[i].Status.ContainerStatuses {
			if !status.Ready {
				continue podIter
			}
		}

		addresses = append(addresses, redis.Address{Host: podList.Items[i].Status.PodIP, Port: strconv.Itoa(redis.Port)})
	}

	// Run Redis Replication Reconfiguration
	replication, err := redis.New(options.password, addresses...)
	if err != nil {
		// This is considered part of normal operation - return and requeue
		logger.Info("Error creating Redis replication, requeue", "error", err)
		return ctrl.Result{Requeue: true}, nil
	}
	defer replication.Disconnect()

	if err := replication.Reconfigure(); err != nil {
		return ctrl.Result{}, fmt.Errorf("error reconfiguring replication: %s", err)
	}

	// Select master and assign the master and replica labels to the corresponding Pods.
	// Wrapping it with the exponential backoff timer in order to wait for the updated info replication.
	var master redis.Address
	exponentialBackOff := backoff.NewExponentialBackOff()
	exponentialBackOff.MaxElapsedTime = redis.DefaultFailoverTimeout

	if err := backoff.Retry(func() error {
		if err := replication.Refresh(); err != nil {
			return err
		}

		if master = replication.GetMasterAddress(); master == (redis.Address{}) {
			return fmt.Errorf("no master discovered")
		}
		return nil
	}, exponentialBackOff); err != nil {
		logger.Info("no master discovered, requeue", "error", err, "replication", replication)
		return ctrl.Result{Requeue: true}, nil
	}

	// update Pod labels asynchronously and fetch the master Pod's name
	var wg sync.WaitGroup
	errChan := make(chan error, len(podList.Items))
	masterChan := make(chan string, 1)

	wg.Add(len(podList.Items))
	for i := range podList.Items {
		go func(pod corev1.Pod, masterAddress string, wg *sync.WaitGroup) {
			defer wg.Done()
			podPatch := client.MergeFrom(pod.DeepCopy())
			if pod.Status.PodIP == masterAddress {
				select {
				case masterChan <- pod.Name:
					if pod.Labels[roleLabelKey] == masterLabel {
						return
					}
					pod.Labels[roleLabelKey] = masterLabel
				default:
					// very unlikely to happen but still...
					errChan <- fmt.Errorf("IP address conflict for pod %s: %s", pod.Name, pod.Status.PodIP)
					return
				}
			} else {
				if pod.Labels[roleLabelKey] == replicaLabel {
					return
				}
				pod.Labels[roleLabelKey] = replicaLabel
			}
			if err := r.Client.Patch(ctx, &pod, podPatch); err != nil {
				errChan <- err
			}
		}(podList.Items[i], master.Host, &wg)
	}
	wg.Wait()

	close(errChan)
	if len(errChan) > 0 {
		var b strings.Builder
		defer b.Reset()
		for err := range errChan {
			if !errors.IsConflict(err) {
				_, _ = fmt.Fprintf(&b, " %s;", err)
			}
		}
		if b.Len() > 0 {
			return ctrl.Result{}, fmt.Errorf("failed to update Pods:%s", b.String())
		}
		loggerDebug("Conflict updating Pods, requeue")
		return ctrl.Result{Requeue: true}, nil
	}

	// update configmap with the current master's IP address
	options.master = master
	if result, err := r.createOrUpdate(ctx, new(corev1.ConfigMap), redisObject, options); err != nil {
		return result, err
	} else if result.Requeue {
		logger.Info("Updated ConfigMap")
		return result, nil
	}

	masterPodName := <-masterChan
	if fetchedRedis.Status.Replicas == replication.Size() && fetchedRedis.Status.Master == masterPodName {
		// Everything is OK - don't requeue
		return ctrl.Result{}, nil
	}

	fetchedRedis.Status.Replicas = replication.Size()
	fetchedRedis.Status.Master = masterPodName
	if err := r.Client.Status().Update(ctx, fetchedRedis); err != nil {
		if errors.IsConflict(err) {
			loggerDebug("Conflict updating Redis status, requeue")
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to update Redis status: %s", err)
	}
	logger.Info("Updated Redis status")
	return ctrl.Result{}, nil
}

// createOrUpdate abstracts away keeping in sync the desired and actual state of Kubernetes objects.
// passing an empty instance implementing runtime.Object will generate the appropriate “expected” object,
// create an object if it does not exist, compare the existing object with the generated one and update if needed.
// the Result.Requeue will be true if the object was successfully created or updated or in case there was a conflict updating the object.
func (r *RedisReconcile) createOrUpdate(
	ctx context.Context,
	object client.Object,
	redis *k8sv1alpha1.Redis,
	options objectGeneratorOptions,
) (result ctrl.Result, err error) {
	generatedObject := generateObject(redis, object, options)
	objectMeta := generatedObject.(metav1.Object)

	if err = r.Client.Get(ctx, types.NamespacedName{
		Namespace: redis.GetNamespace(),
		Name:      objectMeta.GetName(),
	}, object); err != nil {
		if errors.IsNotFound(err) {
			// Set Redis instance as the owner and controller
			if err = controllerutil.SetControllerReference(redis, objectMeta, r.Scheme); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to set owner for Object: %s", err)
			}
			if err = r.Client.Create(ctx, generatedObject); err != nil && !errors.IsAlreadyExists(err) {
				return ctrl.Result{}, fmt.Errorf("failed to create Object: %s", err)
			}
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to fetch Object: %s", err)
	}

	if !objectUpdateNeeded(object, generatedObject) {
		return
	}

	if err = r.Client.Update(ctx, object); err != nil {
		if errors.IsConflict(err) {
			// conflicts can be common, consider it part of normal operation
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to update Object: %s", err)
	}
	return ctrl.Result{Requeue: true}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconcile) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8sv1alpha1.Redis{}).
		Owns(new(corev1.Secret)).
		Owns(new(corev1.Service)).
		Owns(new(corev1.ConfigMap)).
		Owns(new(v1.PodDisruptionBudget)).
		Owns(new(appsv1.StatefulSet)).
		Complete(r)
}

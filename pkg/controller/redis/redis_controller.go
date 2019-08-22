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

package redis

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	k8sv1alpha1 "github.com/amaizfinance/redis-operator/pkg/apis/k8s/v1alpha1"
	"github.com/amaizfinance/redis-operator/pkg/redis"

	"github.com/cenkalti/backoff"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	log = logf.Log.WithName("controller_redis")
	// used to check if the password is a simple alphanumeric string
	isAlphaNumeric = regexp.MustCompile(`^[[:alnum:]]+$`).MatchString
)

// Add creates a new Redis Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRedis{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("redis-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Redis
	if err := c.Watch(
		&source.Kind{Type: new(k8sv1alpha1.Redis)},
		new(handler.EnqueueRequestForObject),
	); err != nil {
		return err
	}

	for _, object := range []runtime.Object{
		new(corev1.Secret),
		new(corev1.Service),
		new(corev1.ConfigMap),
		new(policyv1beta1.PodDisruptionBudget),
		new(appsv1.StatefulSet),
	} {
		if err := c.Watch(
			&source.Kind{Type: object},
			&handler.EnqueueRequestForOwner{OwnerType: new(k8sv1alpha1.Redis), IsController: true},
		); err != nil {
			return err
		}
	}

	return nil
}

// ReconcileRedis reconciles a Redis object
type ReconcileRedis struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// strict implementation check
var _ reconcile.Reconciler = (*ReconcileRedis)(nil)

// Reconcile reads that state of the cluster for a Redis object and makes changes based on the state read
// and what is in the Redis.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (reconciler *ReconcileRedis) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Namespace", request.Namespace, "Redis", request.Name)
	loggerDebug := logger.V(1).Info
	loggerDebug("Reconciling Redis")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Fetch the Redis instance
	fetchedRedis := new(k8sv1alpha1.Redis)
	if err := reconciler.client.Get(ctx, request.NamespacedName, fetchedRedis); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
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
		if err := reconciler.client.Get(ctx, types.NamespacedName{
			Namespace: request.Namespace,
			Name:      redisObject.Spec.Password.SecretKeyRef.Name,
		}, passwordSecret); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to fetch password: %s", err)
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
	for i, object := range []runtime.Object{
		new(corev1.Service), new(corev1.Service), new(corev1.Service), // 3 distinct services ;)
		new(corev1.Secret),
		new(corev1.ConfigMap),
		new(policyv1beta1.PodDisruptionBudget),
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

		if result, err := reconciler.createOrUpdate(ctx, object, redisObject, options); err != nil {
			return reconcile.Result{}, err
		} else if result.Requeue {
			logger.Info(fmt.Sprintf("Applied %#v", object))
			return result, nil
		}
	}

	// all the kubernetes resources are OK.
	// Redis failover state should be checked and reconfigured if needed.
	podList := new(corev1.PodList)
	if err := reconciler.client.List(ctx, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(redisObject.Labels),
		Namespace:     request.Namespace,
	}, podList); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to list Pods: %s", err)
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
		return reconcile.Result{Requeue: true}, nil
	}
	defer replication.Disconnect()

	if err := replication.Reconfigure(); err != nil {
		return reconcile.Result{}, fmt.Errorf("error reconfiguring replication: %s", err)
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
		return reconcile.Result{Requeue: true}, nil
	}

	// update Pod labels asynchronously and fetch the master Pod's name
	var wg sync.WaitGroup
	errChan := make(chan error, len(podList.Items))
	masterChan := make(chan string, 1)

	wg.Add(len(podList.Items))
	for i := range podList.Items {
		go func(pod corev1.Pod, masterAddress string, wg *sync.WaitGroup) {
			defer wg.Done()
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
			if err := reconciler.client.Update(ctx, &pod); err != nil {
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
			return reconcile.Result{}, fmt.Errorf("failed to update Pods:%s", b.String())
		}
		loggerDebug("Conflict updating Pods, requeue")
		return reconcile.Result{Requeue: true}, nil
	}

	// update configmap with the current master's IP address
	options.master = master
	if result, err := reconciler.createOrUpdate(ctx, new(corev1.ConfigMap), redisObject, options); err != nil {
		return result, err
	} else if result.Requeue {
		logger.Info("Updated ConfigMap")
		return result, nil
	}

	masterPodName := <-masterChan
	if fetchedRedis.Status.Replicas == replication.Size() && fetchedRedis.Status.Master == masterPodName {
		// Everything is OK - don't requeue
		return reconcile.Result{}, nil
	}

	fetchedRedis.Status.Replicas = replication.Size()
	fetchedRedis.Status.Master = masterPodName
	if err := reconciler.client.Status().Update(ctx, fetchedRedis); err != nil {
		if errors.IsConflict(err) {
			loggerDebug("Conflict updating Redis status, requeue")
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to update Redis status: %s", err)
	}
	logger.Info("Updated Redis status")
	return reconcile.Result{}, nil
}

// createOrUpdate abstracts away keeping in sync the desired and actual state of Kubernetes objects.
// passing an empty instance implementing runtime.Object will generate the appropriate ``expected'' object,
// create an object if it does not exist, compare the existing object with the generated one and update if needed.
// the Result.Requeue will be true if the object was successfully created or updated or in case there was a conflict updating the object.
func (reconciler *ReconcileRedis) createOrUpdate(
	ctx context.Context,
	object runtime.Object,
	redis *k8sv1alpha1.Redis,
	options objectGeneratorOptions,
) (result reconcile.Result, err error) {
	generatedObject := generateObject(redis, object, options)
	objectMeta := generatedObject.(metav1.Object)

	if err = reconciler.client.Get(ctx, types.NamespacedName{
		Namespace: redis.GetNamespace(),
		Name:      objectMeta.GetName(),
	}, object); err != nil {
		if errors.IsNotFound(err) {
			// Set Redis instance as the owner and controller
			if err = controllerutil.SetControllerReference(redis, objectMeta, reconciler.scheme); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to set owner for Object: %s", err)
			}
			if err = reconciler.client.Create(ctx, generatedObject); err != nil && !errors.IsAlreadyExists(err) {
				return reconcile.Result{}, fmt.Errorf("failed to create Object: %s", err)
			}
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to fetch Object: %s", err)
	}

	if !objectUpdateNeeded(object, generatedObject) {
		return
	}

	if err = reconciler.client.Update(ctx, object); err != nil {
		if errors.IsConflict(err) {
			// conflicts can be common, consider it part of normal operation
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to update Object: %s", err)
	}
	return reconcile.Result{Requeue: true}, nil
}

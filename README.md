# Redis Operator

[![Build Status](https://cloud.drone.io/api/badges/amaizfinance/redis-operator/status.svg)](https://cloud.drone.io/amaizfinance/redis-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/amaizfinance/redis-operator)](https://goreportcard.com/report/github.com/amaizfinance/redis-operator)
[![GolangCI](https://golangci.com/badges/github.com/amaizfinance/redis-operator.svg)](https://golangci.com/r/github.com/amaizfinance/redis-operator)
[![LICENSE](https://img.shields.io/github/license/amaizfinance/redis-operator.svg)](https://github.com/amaizfinance/redis-operator/blob/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/amaizfinance/redis-operator?status.svg)](https://godoc.org/github.com/amaizfinance/redis-operator)
[![Releases](https://img.shields.io/github/release/amaizfinance/redis-operator.svg)](https://github.com/amaizfinance/redis-operator/releases)

## Project status: alpha

The basic features have been completed, and while no breaking API changes are currently planned, the API can change in a backwards incompatible way before the project is declared stable.

## Overview

Redis Operator can be considered a Kubernetes-native replacement for [Redis Sentinel][sentinel]. It creates the [Redis] instances and maintains high availability and automatic failover.

Fundamental things to know about Redis Operator:

* `3` is a minimum number of Redis instances. Having `3` instances allows to always maintain a simple master-replica pair thus making it possible to replicate data even with Redis persistence turned off.
* Redis Operator is stateless. It means that it does not store any information about Redis instances internally. If an instance of the operator terminates in the middle of the failover process it will reconnect to Redis instances and reconfigure them if it is still required.
* Redis Operator is not a distributed system. It leverages a simple leader election protocol. You can run multiple instances of Redis Operator. Detailed description of leader election can be found [here][leader-election].
* One Redis Operator deployment is designed to rule multiple Redis replication setups. However you should bear in mind that current implementation is limited to reconfiguring one Redis replication at a time.
* Redis Operator does not provide continuous monitoring, notification and service discovery. Those are provided by Kubernetes itself.
* Redis clients don't need Sentinel support. Appropriate `role` labels are added to each pod and end users are encouraged to use services to connect to master or replica nodes.
* Redis 5.0 is the minimum supported version.

## Getting Started

### Deploying the Redis operator

1. Create a namespace for the operator:

    ```bash
    kubectl create namespace redis-operator
    ```

2. Create all the necessary resources and deploy the operator:

    ```bash
    kubectl apply -Rf deploy
    ```

3. Verify that the operator is running:

    ```bash
    $ kubectl -n redis-operator get deployment
    NAME             DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
    redis-operator   1         1         1            1           5m
    ```

### Deploying Redis

Redis can be deployed by creating a `Redis` Custom Resource(CR).

1. Create a Redis CR that deploys a 3 node Redis replication in high availablilty mode:

    ```bash
    kubectl apply -f example/k8s_v1alpha1_redis_cr.yaml
    ```

2. Wait until the `redis-example-...` pods for the Redis replication are up and check the status of 'redis'. It will show the name for the Pod of the current master instance and the total number of replicas in the setup:

    ```bash
    $ kubectl get redis example
    NAME      MASTER            REPLICAS   DESIRED   AGE
    example   redis-example-0   3          3         24d
    ```

3. Scale the deployment:

    ```bash
    $ kubectl scale redis example --replicas 4
    redis.k8s.amaiz.com/example scaled
    $ kubectl get redis example
    NAME      MASTER            REPLICAS   DESIRED   AGE
    example   redis-example-0   4          4         24d
    ```

4. Redis Operator creates the following resources owned by the corresponding `Redis` CR. Please note that the name of `Redis` (`example` in this case) is used as an infix or suffix for the names of the generated resources:

    * Secret `redis-example` (in case the password is set up)
    * ConfigMap `redis-example`
    * PodDisruptionBudget `redis-example`
    * StatefulSet `redis-example`
    * Services:
        * `redis-example` - covers all instances
        * `redis-example-headless` - covers all instances, headless
        * `redis-example-master` - service for access to the master instance

### Configuring Redis

All configuration of Redis is done via editing the `Redis` resourse file. Full annotated example can be found in the `examples` directory of the repo.

## Uninstalling Redis operator

Delete the operators and CRDs. Kubernetes will garbage collect all operator-managed resources:

```bash
kubectl delete namespace redis-operator
kubectl delete crd redis.k8s.amaiz.com
```

## Design and goals

The main goal of the project is to create a Kubernetes native service for managing highly available Redis deployments. Sentinel is great for simple Redis replication but it does not fit into Kubernetes model for a number of reasons:

* yet another distributed system to maintain. In order to support automatic failover and high availability of Redis one should figure out some way to support automatic failover and high availability of Sentinel.
* feature overlap. Monitoring(periodic instance health check), notifications(events) and service discovery are something Kubernetes already provides out of the box and can be leveraged by implementing your own controller and Custom Resources.
* Sentinel allows to resist without human intervention _to certain kind of failures_. The goal of the operator is to create a Redis deployment that would resist without human intervention _to most kind of failures_.

Another imporatant goal of this project is to resist failures even with persistence turned off. In some scenarios persisting data on disk is not permitted and all the data should reside only in-memory no matter what. And at the same time losing this data is undesirable.

### Automatic failover algorithm details

Redis Operator is not a distributed system. Instead it leverages the Kuberenetes API to perform a leader election upon startup. Current implementation of the leader election [algorithm][leader-election] precludes the possibility of 2 instances mistakenly running as leaders (split brain).

Redis Operator watches for changes to the `Redis` resource as well as the resources managed by the operator and owned by `Redis`, e.g. `ConfigMaps`, `Secrets`, `Services`, `PodDisruptionBudgets` and `StatefulSets`. Should anything happen to any of the above the operator will check the state of the Kubernetes resources along with the state of Redis replication and reconfigure them if needed.

All the managed resources are created or updated in the first place. The resources already present are always compared to the resources generated by the operator and updated if they differ.

Once all the resources are in sync the list of Redis instances is compiled from the list of `Pod`s owned by the corresponding `StatefulSet`. Only `Pod`s with all containers running and ready are taken into account.

Minimum failover size is `2`. `2` represents a simple master-replica pair essential for running replication. If the number of instances is less than the minimum failover size no reconfiguration will be performed. With this in mind it is absolutely normal to lose all instances but one at the same time. Even with persistence turned off the data will be preserved and replicated across all `Pod`s that come in place of the terminated ones.

Redis Operator is stateless. It means that the state of replication is determined every time the list of Redis instances is analyzed.

The state of replication is determined based on the [`INFO REPLICATION`][info] output of every instance from the list.

A healthy replication is the state when there is a single master and all other instances are connected to it. In this case the operator will do nothing.

Master is an instance with at least one connected replica. If there is no masters found then there is one of two cases met:

* the master is lost. Then there's at least one replica and one of the replicas should be promoted to master
* all instances are masters. This is considered to be the initial state thus any instance can be chosen as a master

In case the master has been lost the list of candidate instances are sorted according to their replica priority and replication offset. Instances with replica priority equal to `0` are filtered out prior to sorting.

A replica with the lowest priority and/or higher replication offset is promoted to master.

With the master in place all other instances that do not report themselves as the master's replicas are reconfigured appropriately. All replicas in question are reconfigured simultaneously.

Once the reconfiguration has been finished all `Pod`s are labeled appropriately with `role=master` or `role=replica` labels. Current master's Pod name and the total quantity of connected instances are written to the status field of the `Redis` resource. The `ConfigMap` is updated with the master's IP address.

[Redis]: https://redis.io
[sentinel]: https://redis.io/topics/sentinel
[leader-election]: https://github.com/operator-framework/operator-sdk/blob/v0.7.0/doc/user-guide.md#leader-election
[info]: https://redis.io/commands/info

## Plans

### Short term

- [ ] add more testing

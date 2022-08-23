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
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v3"
	"github.com/go-redis/redis/v8"
	"github.com/spf13/cast"
)

const (
	// Port is a standard Redis port. Is not meant to change
	Port = 6379

	// MinimumFailoverSize sets the minimum desired size of Redis replication.
	// It reflects a simple master - replica pair.
	// Due to the highly volatile nature of Kubernetes environments
	// it is better to keep at least 3 instances and feel free to lose one instance for whatever reason.
	// It is especially useful for scenarios when there is no need or permission to use persistent storage.
	// In such cases it is safe to run Redis replication failover and the risk of losing data is minimal.
	MinimumFailoverSize = 2

	// Roles as seen in the info replication output
	RoleMaster  = "role:master"
	RoleReplica = "role:slave"

	// master-specific fields
	connectedReplicas = "connected_slaves"
	masterReplOffset  = "master_repl_offset"

	// replica-specific fields
	replicaPriority   = "slave_priority"
	replicationOffset = "slave_repl_offset"
	masterHost        = "master_host"
	masterPort        = "master_port"
	masterLinkStatus  = "master_link_status"

	// DefaultFailoverTimeout sets the maximum timeout for the exponential backoff timer
	DefaultFailoverTimeout = 5 * time.Second
)

var (
	infoReplicationRe = buildInfoReplicationRe()
)

// buildInfoReplicationRe is a helper function to build a regexp for parsing INFO REPLICATION output
func buildInfoReplicationRe() *regexp.Regexp {
	var b strings.Builder
	defer b.Reset()
	// start from setting the multi-line flag
	b.WriteString(`(?m)`)

	// IPv4 address regexp
	addrRe := `((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`

	// templates for simple fields
	numTmpl := `^%s:\d+\s*?$`
	strTmpl := `^%s:\w+\s*?$`

	// build them all up
	for name, tmpl := range map[string]string{
		// master-specific fields
		connectedReplicas: numTmpl,
		masterReplOffset:  numTmpl,

		// replica-specific fields
		replicaPriority:   numTmpl,
		replicationOffset: numTmpl,
		masterHost:        fmt.Sprintf(`^%%s:%s\s*?$`, addrRe),
		masterPort:        numTmpl,
		masterLinkStatus:  strTmpl,
	} {
		_, _ = fmt.Fprintf(&b, tmpl, name)
		_, _ = fmt.Fprint(&b, "|")
	}
	// replica regexp is the most complex of all
	_, _ = fmt.Fprintf(&b, `^slave\d+:ip=%s,port=\d{1,5},state=\w+,offset=\d+,lag=\d+\s*?$`, addrRe)
	return regexp.MustCompile(b.String())
}

// client is an extract of redis.Cmdable
type client interface {
	Ping(ctx context.Context) *redis.StatusCmd
	Info(ctx context.Context, section ...string) *redis.StringCmd
	TxPipelined(ctx context.Context, fn func(redis.Pipeliner) error) ([]redis.Cmder, error)
	Close() error
}

// rediser defines the instance methods
type rediser interface {
	replicaOf(master Address) error
	getInfo() (string, error)
	refresh(info string) error
}

// Replication is the interface for checking the status of replication
type Replication interface {
	// Reconfigure checks the state of replication and reconfigures instances if needed
	Reconfigure() error
	// Size returns the total number of replicas
	Size() int
	// GetMasterAddress returns the current master address
	GetMasterAddress() Address
	// Refresh refreshes replication info for every instance
	Refresh() error
	// Disconnect closes connections to all instances
	Disconnect()

	selectMaster() *instance
	promoteReplicaToMaster() (*instance, error)
	reconfigureAsReplicasOf(master Address) error
}

// Address represents the Host:Port pair of a instance instance
type Address struct {
	Host string
	Port string
}

func (a Address) String() string {
	return fmt.Sprintf("%s:%s", a.Host, a.Port)
}

// strict implementation check
var (
	_ rediser = (*instance)(nil)
)

// instance struct includes a subset of fields returned by INFO
type instance struct {
	Address

	role              string
	replicationOffset int

	// master-specific fields
	connectedReplicas int
	replicas          instances

	// replica-specific fields
	replicaPriority  int
	masterHost       string
	masterPort       string
	masterLinkStatus string

	client client
}

// replicaOf changes the replication settings of a replica on the fly
func (i *instance) replicaOf(master Address) (err error) {
	// promote replica to master
	if master == (Address{}) {
		master.Host = "NO"
		master.Port = "ONE"
	}

	/* In order to send REPLICAOF in a safe way, we send a transaction performing
	 * the following tasks:
	 * 1) Reconfigure the instance according to the specified host/port params.
	 * 2) Disconnect all clients (but this one sending the command) in order
	 *    to trigger the ask-master-on-reconnection protocol for connected
	 *    clients.
	 *
	 * Note that we don't check the replies returned by commands, since we
	 * will observe instead the effects in the next INFO output. */
	ctx := context.TODO()
	_, err = i.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.SlaveOf(ctx, master.Host, master.Port)
		pipe.ClientKillByFilter(ctx, "TYPE", "NORMAL")
		return nil
	})

	return err
}

func (i *instance) getInfo() (info string, err error) {
	info, err = i.client.Info(context.TODO(), "replication").Result()
	if err != nil {
		return "", fmt.Errorf("getting info replication failed for %s: %s", i.Address, err)
	}
	return
}

// refresh parses the instance info and updates the instance fields appropriately
func (i *instance) refresh(info string) error {
	// parse info replication answer
	switch {
	case strings.Contains(info, RoleMaster):
		i.role = RoleMaster
	case strings.Contains(info, RoleReplica):
		i.role = RoleReplica
	default:
		return errors.New("the role is wrong")
	}

	// parse all other attributes
	for _, parsed := range infoReplicationRe.FindAllString(info, -1) {
		switch s := strings.TrimSpace(parsed); {
		// master-specific
		case i.role == RoleMaster && strings.HasPrefix(s, connectedReplicas):
			i.connectedReplicas = cast.ToInt(strings.Split(s, ":")[1])
		case i.role == RoleMaster && strings.HasPrefix(s, masterReplOffset):
			i.replicationOffset = cast.ToInt(strings.Split(s, ":")[1])
		case i.role == RoleMaster && strings.HasPrefix(s, "slave"):
			replica := instance{}
			for _, field := range strings.Split(strings.Split(s, ":")[1], ",") {
				switch {
				case strings.HasPrefix(field, "ip="):
					replica.Host = strings.Split(field, "=")[1]
				case strings.HasPrefix(field, "port="):
					replica.Port = strings.Split(field, "=")[1]
				case strings.HasPrefix(field, "offset="):
					replica.replicationOffset = cast.ToInt(strings.Split(field, "=")[1])
				}
			}
			i.replicas = append(i.replicas, replica)

		// replica-specific
		case i.role == RoleReplica && strings.HasPrefix(s, replicaPriority):
			i.replicaPriority = cast.ToInt(strings.Split(s, ":")[1])
		case i.role == RoleReplica && strings.HasPrefix(s, replicationOffset):
			i.replicationOffset = cast.ToInt(strings.Split(s, ":")[1])
		case i.role == RoleReplica && strings.HasPrefix(s, masterHost):
			i.masterHost = strings.Split(s, ":")[1]
		case i.role == RoleReplica && strings.HasPrefix(s, masterLinkStatus):
			i.masterLinkStatus = strings.Split(s, ":")[1]
		case i.role == RoleReplica && strings.HasPrefix(s, masterPort):
			i.masterPort = strings.Split(s, ":")[1]
		}
	}
	return nil
}

type instances []instance

// sort.Interface implementation for instances.
// Len returns the number of redis instances
func (ins instances) Len() int { return len(ins) }

// Swap swaps the elements with indexes i and j.
func (ins instances) Swap(i, j int) { ins[i], ins[j] = ins[j], ins[i] }

// Less chooses an instance with a lesser priority and higher replication offset.
// Note that this assumes that instances don't have replicas with replicaPriority == 0
func (ins instances) Less(i, j int) bool {
	// choose a replica with less replica priority
	// choose a bigger replication offset otherwise
	if ins[i].replicaPriority == ins[j].replicaPriority {
		return ins[i].replicationOffset > ins[j].replicationOffset
	}
	return ins[i].replicaPriority < ins[j].replicaPriority
}

// Reconfigure checks the state of the instance replication and tries to fix/initially set the state.
// There should be only one master. All other instances should report the same master.
// Working master serves as a source of truth. It means that only those replicas who are not reported by master
// as its replicas will be reconfigured.
func (ins instances) Reconfigure() (err error) {
	// nothing to do here
	if len(ins) == 0 {
		return nil
	}

	master := ins.selectMaster()

	// we've lost the master, promote a replica to master role
	if master == nil {
		var candidates instances
		// filter out non-replicas
		for i := range ins {
			if ins[i].role == RoleReplica && ins[i].replicaPriority != 0 {
				candidates = append(candidates, ins[i])
			}
		}
		master, err = candidates.promoteReplicaToMaster()
		if err != nil {
			return err
		}
	}

	// connectedReplicas will be needed to compile a slice of orphaned(not connected to current master) ins
	connectedReplicas := make(map[Address]struct{})
	for _, replica := range master.replicas {
		connectedReplicas[replica.Address] = struct{}{}
	}

	var replicas instances
	for i := range ins {
		if _, there := connectedReplicas[ins[i].Address]; ins[i].Address != master.Address && !there {
			replicas = append(replicas, ins[i])
		}
	}

	// configure replicas
	return replicas.reconfigureAsReplicasOf(master.Address)
}

// Size returns the number of redis instances
func (ins instances) Size() int { return len(ins) }

func (ins instances) GetMasterAddress() Address {
	if master := ins.selectMaster(); master != nil {
		return master.Address
	}
	return Address{}
}

// Refresh fetches and refreshes info for all instances
func (ins instances) Refresh() error {
	var wg sync.WaitGroup
	instanceCount := len(ins)
	ch := make(chan string, instanceCount)
	wg.Add(instanceCount)

	for i := range ins {
		go func(i *instance, wg *sync.WaitGroup) {
			defer wg.Done()
			info, err := i.getInfo()
			if err != nil {
				ch <- fmt.Sprintf("%s: %s", i.Address, err)
				return
			}
			if err := i.refresh(info); err != nil {
				ch <- fmt.Sprintf("%s: %s", i.Address, err)
				return
			}
		}(&ins[i], &wg)
	}
	wg.Wait()
	close(ch)

	if len(ch) > 0 {
		var b strings.Builder
		defer b.Reset()
		for e := range ch {
			_, _ = fmt.Fprintf(&b, "%s;", e)
		}
		return errors.New(b.String())
	}
	return nil
}

// Disconnect closes the connections and releases the resources
func (ins instances) Disconnect() {
	for i := range ins {
		_ = ins[i].client.Close()
	}
}

// selectMaster chooses any working master in case of a working replication or any other master otherwise.
// Working master in this case is a master with at least one replica connected.
func (ins instances) selectMaster() *instance {
	// normal state. we have a working replication with the master being online
	for _, i := range ins {
		// filter out replicas since they can also have their own replicas...
		if i.role == RoleReplica {
			continue
		}

		// we've found a working master
		if i.connectedReplicas > 0 {
			return &i
		}
	}

	// If we have at least one replica it means
	// we've lost the current master and need to promote a replica to a master
	for _, i := range ins {
		if i.role == RoleReplica && i.replicaPriority != 0 {
			return nil
		}
	}

	// This is supposed to be an initial state.
	// When you roll out a bunch of Redis instances initially they are all standalone masters.
	// In this case we are free to choose the first one.
	if len(ins) > 0 {
		return &ins[0]
	}

	return nil
}

// promoteReplicaToMaster selects a replica for promotion and promotes it to master role
func (ins instances) promoteReplicaToMaster() (*instance, error) {
	sort.Sort(ins)
	promoted := &ins[0]
	exponentialBackOff := backoff.NewExponentialBackOff()
	exponentialBackOff.MaxElapsedTime = DefaultFailoverTimeout

	if err := promoted.replicaOf(Address{}); err != nil {
		return nil, fmt.Errorf("could not promote replica %s to master: %s", promoted.Address, err)
	}

	// promote replica to master and wait until it reports itself as a master
	return promoted, backoff.Retry(func() error {
		info, err := promoted.getInfo()
		if err != nil {
			return err
		}
		if err := promoted.refresh(info); err != nil {
			return err
		}
		if promoted.role != RoleMaster {
			return fmt.Errorf("still waiting for the replica %s to be promoted", promoted.Address)
		}
		return nil
	}, exponentialBackOff)
}

// reconfigureAsReplicasOf configures instances as replicas of the master
func (ins instances) reconfigureAsReplicasOf(master Address) error {
	// do it simultaneously for all replicas
	var wg sync.WaitGroup
	replicasCount := len(ins)
	ch := make(chan string, replicasCount)
	wg.Add(replicasCount)

	for i := range ins {
		go func(replica *instance, wg *sync.WaitGroup) {
			defer wg.Done()

			if err := replica.replicaOf(master); err != nil {
				ch <- fmt.Sprintf("error reconfiguring replica %s: %v", replica.Address, err)
			}
		}(&ins[i], &wg)
	}
	wg.Wait()
	close(ch)

	if len(ch) > 0 {
		var b strings.Builder
		defer b.Reset()
		for e := range ch {
			_, _ = fmt.Fprintf(&b, "%s;", e)
		}
		return errors.New(b.String())
	}
	return nil
}

// New creates a new redis replication.
// Instances are added on the best effort basis. It means that out of N addresses passed
// if at least 2 instances are healthy the replication will be created. Otherwise New will return an error.
func New(password string, addresses ...Address) (Replication, error) {
	instances := make(instances, 0, len(addresses))
	for _, address := range addresses {
		r := instance{
			Address: address,
			client:  redis.NewClient(&redis.Options{Addr: address.String(), Password: password}),
		}

		// check connection and add the instance if Ping succeeds
		if err := r.client.Ping(context.TODO()).Err(); err != nil {
			// TODO: handle -BUSY status
			_ = r.client.Close()
			continue
		}
		instances = append(instances, r)
	}

	if len(instances) < MinimumFailoverSize {
		instances.Disconnect()
		return nil, fmt.Errorf("minimum replication size is not met, only %d are healthy", len(instances))
	}

	if err := instances.Refresh(); err != nil {
		instances.Disconnect()
		return nil, fmt.Errorf("refreshing instance instances info failed: %s", err)
	}

	return instances, nil
}

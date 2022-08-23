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
	"reflect"
	"sort"
	"testing"

	"github.com/go-redis/redis/v8"
)

const (
	masterInfo = `# Replication
role:master
connected_slaves:2
slave0:ip=172.18.0.5,port=6379,state=online,offset=47054,lag=1
slave1:ip=172.18.0.4,port=6379,state=online,offset=47040,lag=1
master_replid:d5cb36eacf068fd6ff3a61c1b7c59192a4db6eaa
master_replid2:0000000000000000000000000000000000000000
master_repl_offset:47054
second_repl_offset:-1
repl_backlog_active:1
repl_backlog_size:1048576
repl_backlog_first_byte_offset:1
repl_backlog_histlen:47054`
	replicaInfo = `# Replication
role:slave
master_host:172.18.0.2
master_port:6379
master_link_status:up
master_last_io_seconds_ago:4
master_sync_in_progress:0
slave_repl_offset:47054
slave_priority:100
slave_read_only:1
connected_slaves:0
master_replid:d5cb36eacf068fd6ff3a61c1b7c59192a4db6eaa
master_replid2:0000000000000000000000000000000000000000
master_repl_offset:47054
second_repl_offset:-1
repl_backlog_active:1
repl_backlog_size:1048576
repl_backlog_first_byte_offset:1
repl_backlog_histlen:47054`
)

func Test_buildInfoReplicationRe(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			"master",
			masterInfo,
			[]string{
				"connected_slaves:2",
				"slave0:ip=172.18.0.5,port=6379,state=online,offset=47054,lag=1",
				"slave1:ip=172.18.0.4,port=6379,state=online,offset=47040,lag=1",
				"master_repl_offset:47054",
			},
		},
		{
			"replica",
			replicaInfo,
			[]string{
				"master_host:172.18.0.2",
				"master_port:6379",
				"master_link_status:up",
				"slave_repl_offset:47054",
				"slave_priority:100",
				"connected_slaves:0",
				"master_repl_offset:47054",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildInfoReplicationRe().FindAllString(tt.input, -1); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildInfoReplicationRe() = %v, want %v", got, tt.want)
			}
		})
	}
}
func TestRedises_Sort(t *testing.T) {
	tests := []struct {
		name string
		have instances
		want instances
	}{
		{"empty", instances{}, instances{}},
		{
			"unchanged",
			instances{
				instance{replicationOffset: 1238, replicaPriority: 100},
				instance{replicationOffset: 1238, replicaPriority: 100},
				instance{replicationOffset: 1236, replicaPriority: 100},
				instance{replicationOffset: 1234, replicaPriority: 100},
			},
			instances{
				instance{replicationOffset: 1238, replicaPriority: 100},
				instance{replicationOffset: 1238, replicaPriority: 100},
				instance{replicationOffset: 1236, replicaPriority: 100},
				instance{replicationOffset: 1234, replicaPriority: 100},
			},
		},
		{
			"sortByReplicationOffset",
			instances{
				instance{replicationOffset: 100},
				instance{replicationOffset: 12},
				instance{replicationOffset: 0},
				instance{replicationOffset: 1212},
			},
			instances{
				instance{replicationOffset: 1212},
				instance{replicationOffset: 100},
				instance{replicationOffset: 12},
				instance{replicationOffset: 0},
			},
		},
		{
			"sortBySlavePriority",
			instances{
				instance{replicationOffset: 100, replicaPriority: 100},
				instance{replicationOffset: 12, replicaPriority: 100},
				instance{replicationOffset: 0, replicaPriority: 10},
				instance{replicationOffset: 1212, replicaPriority: 100},
			},
			instances{
				instance{replicationOffset: 0, replicaPriority: 10},
				instance{replicationOffset: 1212, replicaPriority: 100},
				instance{replicationOffset: 100, replicaPriority: 100},
				instance{replicationOffset: 12, replicaPriority: 100},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.Sort(tt.have)
			if !reflect.DeepEqual(tt.have, tt.want) {
				t.Errorf("instances.Sort() = %v, want %v", tt.have, tt.want)
			}
		})
	}
}

func TestRedis_refresh(t *testing.T) {
	tests := []struct {
		name    string
		info    string
		want    *instance
		wantErr bool
	}{
		{"err", "role:err", &instance{}, true},
		{
			"master",
			masterInfo,
			&instance{
				role:              RoleMaster,
				replicationOffset: 47054,
				connectedReplicas: 2,
				replicas: instances{
					instance{
						Address:           Address{"172.18.0.5", "6379"},
						replicationOffset: 47054,
					},
					instance{
						Address:           Address{"172.18.0.4", "6379"},
						replicationOffset: 47040,
					},
				},
			},
			false,
		},
		{
			"replica",
			replicaInfo,
			&instance{
				role:              RoleReplica,
				replicationOffset: 47054,
				replicaPriority:   100,
				masterHost:        "172.18.0.2",
				masterPort:        "6379",
				masterLinkStatus:  "up",
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &instance{}
			if err := r.refresh(tt.info); (err != nil) != tt.wantErr {
				t.Errorf("instance.refresh()\nerror: %v\nwantErr: %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(r, tt.want) {
				t.Errorf("instance.refresh()\nhave: %+v\nwant: %+v", r, tt.want)
			}
		})
	}
}

func TestRedises_SelectMaster(t *testing.T) {
	tests := []struct {
		name      string
		instances instances
		want      *instance
	}{
		{"empty", instances{}, nil},
		{
			"initial setup",
			instances{
				instance{
					Address: Address{"172.18.0.5", "6379"},
					role:    RoleMaster,
				},
				instance{
					Address: Address{"172.18.0.6", "6379"},
					role:    RoleMaster,
				},
				instance{
					Address: Address{"172.18.0.7", "6379"},
					role:    RoleMaster,
				},
			},
			&instance{
				Address: Address{"172.18.0.5", "6379"},
				role:    RoleMaster,
			},
		},
		{
			"master lost",
			instances{
				instance{
					Address: Address{"172.18.0.5", "6379"},
					role:    RoleMaster,
				},
				instance{
					Address: Address{"172.18.0.6", "6379"},
					role:    RoleMaster,
				},
				instance{
					Address: Address{"172.18.0.7", "6379"},
					role:    RoleMaster,
				},
				instance{
					role:              RoleReplica,
					replicationOffset: 47054,
					replicaPriority:   100,
					masterHost:        "172.18.0.2",
					masterPort:        "6379",
					masterLinkStatus:  "up",
				},
			},
			nil,
		},
		{
			"working master present",
			instances{
				instance{
					Address: Address{"172.18.0.5", "6379"},
					role:    RoleMaster,
				},
				instance{
					Address: Address{"172.18.0.6", "6379"},
					role:    RoleMaster,
				},
				instance{
					Address: Address{"172.18.0.7", "6379"},
					role:    RoleMaster,
				},
				instance{
					role:              RoleMaster,
					replicationOffset: 47054,
					connectedReplicas: 2,
					replicas: instances{
						instance{
							Address:           Address{"172.18.0.5", "6379"},
							replicationOffset: 47054,
						},
						instance{
							Address:           Address{"172.18.0.4", "6379"},
							replicationOffset: 47040,
						},
					},
				},
			},
			&instance{
				role:              RoleMaster,
				replicationOffset: 47054,
				connectedReplicas: 2,
				replicas: instances{
					instance{
						Address:           Address{"172.18.0.5", "6379"},
						replicationOffset: 47054,
					},
					instance{
						Address:           Address{"172.18.0.4", "6379"},
						replicationOffset: 47040,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.instances.selectMaster(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("instances.selectMaster()\nhave: %v\nwant: %v", got, tt.want)
			}
		})
	}
}

func TestRedises_Reconfigure(t *testing.T) {
	tests := []struct {
		name      string
		instances instances
		wantErr   bool
	}{
		{"empty", instances{}, false},
		{
			"normally working",
			instances{
				instance{
					role:              RoleMaster,
					replicationOffset: 47054,
					connectedReplicas: 2,
					replicas: instances{
						instance{
							Address:           Address{"172.18.0.5", "6379"},
							replicationOffset: 47054,
						},
						instance{
							Address:           Address{"172.18.0.4", "6379"},
							replicationOffset: 47040,
						},
					},
				},
				instance{
					Address:           Address{"172.18.0.5", "6379"},
					replicationOffset: 47054,
				},
				instance{
					Address:           Address{"172.18.0.4", "6379"},
					replicationOffset: 47040,
				},
			},
			false,
		},
		{
			"new replica discovered",
			instances{
				instance{
					role:              RoleMaster,
					replicationOffset: 47054,
					connectedReplicas: 2,
					replicas: instances{
						instance{
							Address:           Address{"172.18.0.5", "6379"},
							replicationOffset: 47054,
						},
						instance{
							Address:           Address{"172.18.0.4", "6379"},
							replicationOffset: 47040,
						},
					},
				},
				instance{
					Address:           Address{"172.18.0.5", "6379"},
					replicationOffset: 47054,
				},
				instance{
					Address:           Address{"172.18.0.4", "6379"},
					replicationOffset: 47040,
				},
				instance{
					Address:           Address{"172.18.0.6", "6379"},
					role:              RoleReplica,
					replicationOffset: 47054,
					replicaPriority:   100,
					masterHost:        "172.18.0.2",
					masterPort:        "6379",
					masterLinkStatus:  "up",
					client: redis.NewClient(&redis.Options{
						Addr: "192.0.2.1:6379",
					}),
				},
			},
			true,
		},
		{
			"all replicas - intended to fail",
			instances{
				instance{
					Address:           Address{"172.18.0.5", "6379"},
					role:              RoleReplica,
					replicationOffset: 47054,
					replicaPriority:   100,
					masterHost:        "172.18.0.2",
					masterPort:        "6379",
					masterLinkStatus:  "up",
					client: redis.NewClient(&redis.Options{
						Addr: "192.0.2.1:6379",
					}),
				},
				instance{
					Address:           Address{"172.18.0.4", "6379"},
					role:              RoleReplica,
					replicationOffset: 47040,
					replicaPriority:   100,
					masterHost:        "172.18.0.2",
					masterPort:        "6379",
					masterLinkStatus:  "up",
					client: redis.NewClient(&redis.Options{
						Addr: "192.0.2.1:6379",
					}),
				},
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.instances.Reconfigure(); (err != nil) != tt.wantErr {
				t.Errorf("instances.Reconfigure() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRedises_Disconnect(t *testing.T) {
	tests := []struct {
		name      string
		instances instances
	}{
		{"empty", instances{}},
		{
			"some instances",
			instances{
				instance{
					client: redis.NewClient(&redis.Options{
						Addr: "192.0.2.1:6379",
					}),
				},
				instance{
					client: redis.NewClient(&redis.Options{
						Addr: "192.0.2.1:6378",
					}),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.instances.Disconnect()
		})
	}
}

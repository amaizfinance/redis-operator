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

	"github.com/go-redis/redis"
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
		have Redises
		want Redises
	}{
		{"empty", Redises{}, Redises{}},
		{
			"unchanged",
			Redises{
				Redis{ReplicationOffset: 1238, ReplicaPriority: 100},
				Redis{ReplicationOffset: 1238, ReplicaPriority: 100},
				Redis{ReplicationOffset: 1236, ReplicaPriority: 100},
				Redis{ReplicationOffset: 1234, ReplicaPriority: 100},
			},
			Redises{
				Redis{ReplicationOffset: 1238, ReplicaPriority: 100},
				Redis{ReplicationOffset: 1238, ReplicaPriority: 100},
				Redis{ReplicationOffset: 1236, ReplicaPriority: 100},
				Redis{ReplicationOffset: 1234, ReplicaPriority: 100},
			},
		},
		{
			"sortByReplicationOffset",
			Redises{
				Redis{ReplicationOffset: 100},
				Redis{ReplicationOffset: 12},
				Redis{ReplicationOffset: 0},
				Redis{ReplicationOffset: 1212},
			},
			Redises{
				Redis{ReplicationOffset: 1212},
				Redis{ReplicationOffset: 100},
				Redis{ReplicationOffset: 12},
				Redis{ReplicationOffset: 0},
			},
		},
		{
			"sortBySlavePriority",
			Redises{
				Redis{ReplicationOffset: 100, ReplicaPriority: 100},
				Redis{ReplicationOffset: 12, ReplicaPriority: 100},
				Redis{ReplicationOffset: 0, ReplicaPriority: 10},
				Redis{ReplicationOffset: 1212, ReplicaPriority: 100},
			},
			Redises{
				Redis{ReplicationOffset: 0, ReplicaPriority: 10},
				Redis{ReplicationOffset: 1212, ReplicaPriority: 100},
				Redis{ReplicationOffset: 100, ReplicaPriority: 100},
				Redis{ReplicationOffset: 12, ReplicaPriority: 100},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.Sort(tt.have)
			if !reflect.DeepEqual(tt.have, tt.want) {
				t.Errorf("Redises.Sort() = %v, want %v", tt.have, tt.want)
			}
		})
	}
}

func TestRedis_refresh(t *testing.T) {
	tests := []struct {
		name    string
		info    string
		want    *Redis
		wantErr bool
	}{
		{"err", "role:err", &Redis{}, true},
		{
			"master",
			masterInfo,
			&Redis{
				Role:              RoleMaster,
				ReplicationOffset: 47054,
				ConnectedReplicas: 2,
				Replicas: Redises{
					Redis{
						Address:           Address{"172.18.0.5", "6379"},
						ReplicationOffset: 47054,
					},
					Redis{
						Address:           Address{"172.18.0.4", "6379"},
						ReplicationOffset: 47040,
					},
				},
			},
			false,
		},
		{
			"replica",
			replicaInfo,
			&Redis{
				Role:              RoleReplica,
				ReplicationOffset: 47054,
				ReplicaPriority:   100,
				MasterHost:        "172.18.0.2",
				MasterPort:        "6379",
				MasterLinkStatus:  "up",
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Redis{}
			if err := r.refresh(tt.info); (err != nil) != tt.wantErr {
				t.Errorf("Redis.refresh()\nerror: %v\nwantErr: %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(r, tt.want) {
				t.Errorf("Redis.refresh()\nhave: %+v\nwant: %+v", r, tt.want)
			}
		})
	}
}

func TestRedises_SelectMaster(t *testing.T) {
	tests := []struct {
		name      string
		instances Redises
		want      *Redis
	}{
		{"empty", Redises{}, nil},
		{
			"initial setup",
			Redises{
				Redis{
					Address: Address{"172.18.0.5", "6379"},
					Role:    RoleMaster,
				},
				Redis{
					Address: Address{"172.18.0.6", "6379"},
					Role:    RoleMaster,
				},
				Redis{
					Address: Address{"172.18.0.7", "6379"},
					Role:    RoleMaster,
				},
			},
			&Redis{
				Address: Address{"172.18.0.5", "6379"},
				Role:    RoleMaster,
			},
		},
		{
			"master lost",
			Redises{
				Redis{
					Address: Address{"172.18.0.5", "6379"},
					Role:    RoleMaster,
				},
				Redis{
					Address: Address{"172.18.0.6", "6379"},
					Role:    RoleMaster,
				},
				Redis{
					Address: Address{"172.18.0.7", "6379"},
					Role:    RoleMaster,
				},
				Redis{
					Role:              RoleReplica,
					ReplicationOffset: 47054,
					ReplicaPriority:   100,
					MasterHost:        "172.18.0.2",
					MasterPort:        "6379",
					MasterLinkStatus:  "up",
				},
			},
			nil,
		},
		{
			"working master present",
			Redises{
				Redis{
					Address: Address{"172.18.0.5", "6379"},
					Role:    RoleMaster,
				},
				Redis{
					Address: Address{"172.18.0.6", "6379"},
					Role:    RoleMaster,
				},
				Redis{
					Address: Address{"172.18.0.7", "6379"},
					Role:    RoleMaster,
				},
				Redis{
					Role:              RoleMaster,
					ReplicationOffset: 47054,
					ConnectedReplicas: 2,
					Replicas: Redises{
						Redis{
							Address:           Address{"172.18.0.5", "6379"},
							ReplicationOffset: 47054,
						},
						Redis{
							Address:           Address{"172.18.0.4", "6379"},
							ReplicationOffset: 47040,
						},
					},
				},
			},
			&Redis{
				Role:              RoleMaster,
				ReplicationOffset: 47054,
				ConnectedReplicas: 2,
				Replicas: Redises{
					Redis{
						Address:           Address{"172.18.0.5", "6379"},
						ReplicationOffset: 47054,
					},
					Redis{
						Address:           Address{"172.18.0.4", "6379"},
						ReplicationOffset: 47040,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.instances.SelectMaster(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Redises.SelectMaster()\nhave: %v\nwant: %v", got, tt.want)
			}
		})
	}
}

func TestRedises_Reconfigure(t *testing.T) {
	tests := []struct {
		name      string
		instances Redises
		wantErr   bool
	}{
		{"empty", Redises{}, false},
		{
			"normally working",
			Redises{
				Redis{
					Role:              RoleMaster,
					ReplicationOffset: 47054,
					ConnectedReplicas: 2,
					Replicas: Redises{
						Redis{
							Address:           Address{"172.18.0.5", "6379"},
							ReplicationOffset: 47054,
						},
						Redis{
							Address:           Address{"172.18.0.4", "6379"},
							ReplicationOffset: 47040,
						},
					},
				},
				Redis{
					Address:           Address{"172.18.0.5", "6379"},
					ReplicationOffset: 47054,
				},
				Redis{
					Address:           Address{"172.18.0.4", "6379"},
					ReplicationOffset: 47040,
				},
			},
			false,
		},
		{
			"new replica discovered",
			Redises{
				Redis{
					Role:              RoleMaster,
					ReplicationOffset: 47054,
					ConnectedReplicas: 2,
					Replicas: Redises{
						Redis{
							Address:           Address{"172.18.0.5", "6379"},
							ReplicationOffset: 47054,
						},
						Redis{
							Address:           Address{"172.18.0.4", "6379"},
							ReplicationOffset: 47040,
						},
					},
				},
				Redis{
					Address:           Address{"172.18.0.5", "6379"},
					ReplicationOffset: 47054,
				},
				Redis{
					Address:           Address{"172.18.0.4", "6379"},
					ReplicationOffset: 47040,
				},
				Redis{
					Address:           Address{"172.18.0.6", "6379"},
					Role:              RoleReplica,
					ReplicationOffset: 47054,
					ReplicaPriority:   100,
					MasterHost:        "172.18.0.2",
					MasterPort:        "6379",
					MasterLinkStatus:  "up",
					conn: redis.NewClient(&redis.Options{
						Addr: "192.0.2.1:6379",
					}),
				},
			},
			true,
		},
		{
			"all replicas - intended to fail",
			Redises{
				Redis{
					Address:           Address{"172.18.0.5", "6379"},
					Role:              RoleReplica,
					ReplicationOffset: 47054,
					ReplicaPriority:   100,
					MasterHost:        "172.18.0.2",
					MasterPort:        "6379",
					MasterLinkStatus:  "up",
					conn: redis.NewClient(&redis.Options{
						Addr: "192.0.2.1:6379",
					}),
				},
				Redis{
					Address:           Address{"172.18.0.4", "6379"},
					Role:              RoleReplica,
					ReplicationOffset: 47040,
					ReplicaPriority:   100,
					MasterHost:        "172.18.0.2",
					MasterPort:        "6379",
					MasterLinkStatus:  "up",
					conn: redis.NewClient(&redis.Options{
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
				t.Errorf("Redises.Reconfigure() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRedises_Disconnect(t *testing.T) {
	tests := []struct {
		name      string
		instances Redises
	}{
		{"empty", Redises{}},
		{
			"some instances",
			Redises{
				Redis{
					conn: redis.NewClient(&redis.Options{
						Addr: "192.0.2.1:6379",
					}),
				},
				Redis{
					conn: redis.NewClient(&redis.Options{
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

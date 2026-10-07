package cluster

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

func pod(name, shard, inst string, ready bool, restarts int32) corev1.Pod {
	cond := corev1.ConditionFalse
	if ready {
		cond = corev1.ConditionTrue
	}
	return corev1.Pod{
		Name:              name,
		Labels:            map[string]string{shardIDLabel: shard, instanceLabel: inst},
		CreationTimestamp: metav1.NewTime(time.Now().Add(-90 * time.Second)),
		Spec:              corev1.PodSpec{Containers: []corev1.Container{{Image: "img:tag"}}},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: cond}},
			ContainerStatuses: []corev1.ContainerStatus{{RestartCount: restarts}},
		},
	}
}

func TestBuildStatusGroupsPodsByShard(t *testing.T) {
	pods := []corev1.Pod{
		pod("gameplay-2-dpl-x", "gameplay", "gameplay-2", false, 1),
		pod("gameplay-dpl-y", "gameplay", "gameplay", true, 0),
		pod("lobby-dpl-z", "lobby", "lobby", true, 0),
		{Name: "unlabelled"},
	}
	got := buildStatus(pods, time.Now())
	if len(got) != 2 || got[0].ShardID != "gameplay" || got[1].ShardID != "lobby" {
		t.Fatalf("pools = %+v", got)
	}
	g := got[0]
	if g.PoolSize != 2 || g.Phase != "Progressing" || g.Image != "img:tag" {
		t.Fatalf("gameplay = %+v", g)
	}
	if g.Instances[0].Name != "gameplay" || g.Instances[1].Name != "gameplay-2" || g.Instances[1].RestartCount != 1 {
		t.Fatalf("instances = %+v", g.Instances)
	}
	if got[1].Phase != "Ready" || got[1].Instances[0].Runtime != "lobby-dpl-z" || got[1].Instances[0].Age == "" {
		t.Fatalf("lobby = %+v", got[1])
	}
}

func TestWantInstanceFilters(t *testing.T) {
	type opts = struct{ shards, instances []string }
	cases := []struct {
		o    opts
		want bool
	}{
		{opts{}, true},
		{opts{shards: []string{"gameplay"}}, true},
		{opts{shards: []string{"lobby"}}, false},
		{opts{instances: []string{"gameplay-2"}}, true},
		{opts{instances: []string{"gameplay"}}, false},
	}
	for _, c := range cases {
		got := wantInstance(logsOpts(c.o.shards, c.o.instances), "gameplay", "gameplay-2")
		if got != c.want {
			t.Errorf("%+v: got %v", c.o, got)
		}
	}
	if ts, rest := splitTimestamp(
		"2026-01-01T00:00:00Z hello world",
	); ts != "2026-01-01T00:00:00Z" ||
		rest != "hello world" {
		t.Fatalf("splitTimestamp = %q %q", ts, rest)
	}
}

func logsOpts(shards, instances []string) worldstatus.LogsOpts {
	return worldstatus.LogsOpts{ShardIDs: shards, InstanceNames: instances}
}

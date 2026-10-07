package cluster

import (
	"context"
	"sort"
	"time"

	"github.com/rotisserie/eris"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/argus-labs/world-engine/cli/pkg/worldstatus"
)

// Status lists the shard pools in a namespace from their pods (one pod per instance).
func (c *Client) Status(ctx context.Context, namespace string) ([]worldstatus.PoolStatus, error) {
	cs, err := c.kube(ctx)
	if err != nil {
		return nil, err
	}
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: shardIDLabel})
	if err != nil {
		return nil, eris.Wrapf(err, "list shard pods in %s", namespace)
	}
	return buildStatus(pods.Items, time.Now()), nil
}

func buildStatus(pods []corev1.Pod, now time.Time) []worldstatus.PoolStatus {
	byShard := map[string]*worldstatus.PoolStatus{}
	for i := range pods {
		p := &pods[i]
		shard := p.Labels[shardIDLabel]
		if shard == "" {
			continue
		}
		ps, ok := byShard[shard]
		if !ok {
			ps = &worldstatus.PoolStatus{ShardID: shard}
			byShard[shard] = ps
		}
		if len(p.Spec.Containers) > 0 && ps.Image == "" {
			ps.Image = p.Spec.Containers[0].Image
		}
		ps.Instances = append(ps.Instances, worldstatus.InstanceStatus{
			Name:         p.Labels[instanceLabel],
			Runtime:      p.Name,
			Phase:        string(p.Status.Phase),
			Ready:        podReady(p),
			RestartCount: restartCount(p),
			Age:          duration.HumanDuration(now.Sub(p.CreationTimestamp.Time)),
		})
	}
	out := make([]worldstatus.PoolStatus, 0, len(byShard))
	for _, ps := range byShard {
		sort.Slice(ps.Instances, func(i, j int) bool { return ps.Instances[i].Name < ps.Instances[j].Name })
		ps.PoolSize = int32(len(ps.Instances))
		ready := 0
		for _, in := range ps.Instances {
			if in.Ready {
				ready++
			}
		}
		ps.Phase = "Progressing"
		if ready == len(ps.Instances) {
			ps.Phase = "Ready"
		}
		out = append(out, *ps)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ShardID < out[j].ShardID })
	return out
}

func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func restartCount(p *corev1.Pod) int32 {
	var n int32
	for _, cs := range p.Status.ContainerStatuses {
		n += cs.RestartCount
	}
	return n
}

package scheduler

import (
	"sync"
)

type Client struct {
	sync.Mutex
	counter int
}

type ClientSets []*Client

func (cs ClientSets) Len() int { return len(cs) }

func (cs ClientSets) Less(i, j int) bool {
	return cs[i].counter < cs[j].counter
}
func (cs ClientSets) Swap(i, j int) {
	cs[i], cs[j] = cs[j], cs[i]
}

type Information struct {
	CPU int
}

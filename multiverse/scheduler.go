package multiverse

import (
	"container/heap"
	"container/ring"
	"time"

	"github.com/iotaledger/hive.go/events"
	"github.com/iotaledger/multivers-simulation/config"
	"github.com/iotaledger/multivers-simulation/network"
)

// region Scheduler Interface //////////////////////////////////////////////////////////////////////////////////////////////

// Priority Queue for Message
type PriorityQueue []Message

type IssuerQueue []Message

type BurnPolicyType int

const (
	NoBurn   BurnPolicyType = 0
	Anxious  BurnPolicyType = 1
	Greedy1  BurnPolicyType = 2
	Greedy10 BurnPolicyType = 3
)

type Scheduler interface {
	Setup()
	IncrementAccessMana(float64)
	DecreaseNodeAccessMana(network.PeerID, float64) float64
	BurnValue(time.Time) (float64, bool)
	EnqueueMessage(MessageID)
	ScheduleMessage()
	Events() *SchedulerEvents
	ReadyLen() int
	NonReadyLen() int
	GetNodeAccessMana(network.PeerID) float64
	GetMaxManaBurn() float64
	IssuerQueueLen(network.PeerID) int
	Deficit(network.PeerID) float64
	RateSetter() bool
}

func NewScheduler(tangle *Tangle) (s Scheduler) {
	if config.Params.SchedulerType == "ManaBurn" {
		readyHeap := &PriorityQueue{}
		heap.Init(readyHeap)
		s = &MBScheduler{
			tangle:      tangle,
			readyQueue:  readyHeap,
			nonReadyMap: make(map[MessageID]*Message),
			accessMana:  make(map[network.PeerID]float64, config.Params.NodesCount),
			events: &SchedulerEvents{
				MessageScheduled: events.NewEvent(messageIDEventCaller),
				MessageDropped:   events.NewEvent(messageIDEventCaller),
				MessageEnqueued:  events.NewEvent(schedulerEventCaller),
			},
		}
	} else if config.Params.SchedulerType == "ICCA+" {
		s = &ICCAScheduler{
			tangle:       tangle,
			nonReadyMap:  make(map[MessageID]*Message),
			accessMana:   make(map[network.PeerID]float64, config.Params.NodesCount),
			deficits:     make(map[network.PeerID]float64, config.Params.NodesCount),
			quanta:       make(map[network.PeerID]float64, config.Params.NodesCount),
			issuerQueues: make(map[network.PeerID]*IssuerQueue, config.Params.NodesCount),
			roundRobin:   ring.New(config.Params.NodesCount),
			events: &SchedulerEvents{
				MessageScheduled: events.NewEvent(messageIDEventCaller),
				MessageDropped:   events.NewEvent(messageIDEventCaller),
				MessageEnqueued:  events.NewEvent(schedulerEventCaller),
			},
		}
	} else {
		s = &NoScheduler{
			tangle: tangle,

			events: &SchedulerEvents{
				MessageScheduled: events.NewEvent(messageIDEventCaller),
			},
		}
	}
	return
}

// / region Priority Queue ////////////////////////////////////////////////////////////////////////////////
func (h PriorityQueue) Len() int { return len(h) }
func (h PriorityQueue) Less(i, j int) bool {
	if h[i].ManaBurnValue > h[j].ManaBurnValue {
		return true
	} else if h[i].ManaBurnValue == h[j].ManaBurnValue {
		return float64(h[i].IssuanceTime.Nanosecond()) < float64(h[j].IssuanceTime.Nanosecond())
	} else {
		return false
	}
}
func (h PriorityQueue) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *PriorityQueue) Push(m any) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, m.(Message))
}

func (h *PriorityQueue) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (h PriorityQueue) tail() (tail int) {
	for i := range h {
		if !h.Less(i, tail) { // less means more mana burned/older issue time
			tail = i
		}
	}
	return
}

// region Issuer Queue ////////////////////////////////////////////////////////////////////////////////
func (h IssuerQueue) Len() int { return len(h) }
func (h IssuerQueue) Less(i, j int) bool {
	return float64(h[i].IssuanceTime.Nanosecond()) < float64(h[j].IssuanceTime.Nanosecond())
}
func (h IssuerQueue) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *IssuerQueue) Push(m any) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, m.(Message))
}

func (h *IssuerQueue) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Scheduler Events ///////////////////////////////////////////////////////////////////////////////////////////////////////////

type SchedulerEvents struct {
	MessageScheduled *events.Event
	MessageDropped   *events.Event
	MessageEnqueued  *events.Event
}

func schedulerEventCaller(handler interface{}, params ...interface{}) {
	handler.(func(int, int))(params[0].(int), params[1].(int))
}

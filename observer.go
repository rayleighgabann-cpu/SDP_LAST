package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// EventType lets listeners subscribe to exactly the events they care about.
type EventType string

const (
	EvTransaction EventType = "transaction"
	EvRateChange  EventType = "rate_change"
	EvFraudAlert  EventType = "fraud_alert"
	EvAggregated  EventType = "aggregated"
	EvInspection  EventType = "inspection"
)

// Event is immutable: created once, never modified.
type Event struct {
	Type    EventType
	Payload any
	Created time.Time
}

func NewEvent(t EventType, payload any) Event {
	return Event{Type: t, Payload: payload, Created: time.Now()}
}

type Observer interface{ Handle(Event) }

type Subject interface {
	Subscribe(EventType, Observer)
	Unsubscribe(EventType, Observer)
	Publish(Event)
}

// EventBus is a thread-safe Subject. Publish copies the subscriber slice
// under a read lock and runs handlers outside the lock, so listeners may
// join or leave at runtime - even from inside their own Handle.
type EventBus struct {
	mu   sync.RWMutex
	subs map[EventType][]Observer
}

func NewEventBus() *EventBus {
	return &EventBus{subs: map[EventType][]Observer{}}
}

func (b *EventBus) Subscribe(t EventType, o Observer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[t] = append(b.subs[t], o)
}

func (b *EventBus) Unsubscribe(t EventType, o Observer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	kept := make([]Observer, 0, len(b.subs[t]))
	for _, sub := range b.subs[t] {
		if sub != o {
			kept = append(kept, sub)
		}
	}
	b.subs[t] = kept
}

func (b *EventBus) Publish(e Event) {
	b.mu.RLock()
	snapshot := make([]Observer, len(b.subs[e.Type]))
	copy(snapshot, b.subs[e.Type])
	b.mu.RUnlock()

	for _, o := range snapshot {
		o.Handle(e)
	}
}

// Transaction is the payload of the transaction feed.
type Transaction struct {
	ID     string
	Client string
	Amount float64
}

// Aggregated is the payload republished by the chain listener.
type Aggregated struct {
	Sum   float64
	Count int
}

// FraudDetector flags transactions above a threshold and republishes them
// as fraud alerts on the bus it was constructed with.
type FraudDetector struct {
	threshold float64
	bus       Subject
	mu        sync.Mutex
	flagged   []Transaction
}

func NewFraudDetector(threshold float64, bus Subject) *FraudDetector {
	return &FraudDetector{threshold: threshold, bus: bus}
}

func (f *FraudDetector) Handle(e Event) {
	tx, ok := e.Payload.(Transaction)
	if !ok || tx.Amount <= f.threshold {
		return
	}
	f.mu.Lock()
	f.flagged = append(f.flagged, tx)
	f.mu.Unlock()
	f.bus.Publish(NewEvent(EvFraudAlert, tx))
}

func (f *FraudDetector) FlaggedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.flagged)
}

// BalanceNotifier counts notifications lock-free.
type BalanceNotifier struct{ count int64 }

func NewBalanceNotifier() *BalanceNotifier { return &BalanceNotifier{} }
func (n *BalanceNotifier) Handle(Event)    { atomic.AddInt64(&n.count, 1) }
func (n *BalanceNotifier) Count() int64    { return atomic.LoadInt64(&n.count) }

// LimitMonitor accumulates per-client spending and reports over-limit clients.
type LimitMonitor struct {
	limit float64
	mu    sync.Mutex
	spent map[string]float64
}

func NewLimitMonitor(limit float64) *LimitMonitor {
	return &LimitMonitor{limit: limit, spent: map[string]float64{}}
}

func (m *LimitMonitor) Handle(e Event) {
	tx, ok := e.Payload.(Transaction)
	if !ok {
		return
	}
	m.mu.Lock()
	m.spent[tx.Client] += tx.Amount
	m.mu.Unlock()
}

func (m *LimitMonitor) OverLimit() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for client, spent := range m.spent {
		if spent > m.limit {
			out = append(out, client)
		}
	}
	return out
}

// AuditLog records every event it receives.
type AuditLog struct {
	mu      sync.Mutex
	entries []string
}

func NewAuditLog() *AuditLog { return &AuditLog{} }

func (a *AuditLog) Handle(e Event) {
	a.mu.Lock()
	a.entries = append(a.entries, fmt.Sprintf("%s %s", e.Created.Format("15:04:05"), e.Type))
	a.mu.Unlock()
}

func (a *AuditLog) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.entries)
}

// AggregatorListener is an observer AND a subject: it republishes a running
// total as a new event, forming a chain of observers.
type AggregatorListener struct {
	downstream Subject
	mu         sync.Mutex
	sum        float64
	count      int
}

func NewAggregatorListener(downstream Subject) *AggregatorListener {
	return &AggregatorListener{downstream: downstream}
}

func (a *AggregatorListener) Handle(e Event) {
	tx, ok := e.Payload.(Transaction)
	if !ok {
		return
	}
	a.mu.Lock()
	a.sum += tx.Amount
	a.count++
	agg := Aggregated{Sum: a.sum, Count: a.count}
	a.mu.Unlock()

	a.downstream.Publish(NewEvent(EvAggregated, agg))
}

func (a *AggregatorListener) Sum() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sum
}

// RateTracker listens to the second subject (the rate feed).
type RateTracker struct {
	mu    sync.Mutex
	rates []float64
}

func NewRateTracker() *RateTracker { return &RateTracker{} }

func (r *RateTracker) Handle(e Event) {
	rate, ok := e.Payload.(float64)
	if !ok {
		return
	}
	r.mu.Lock()
	r.rates = append(r.rates, rate)
	r.mu.Unlock()
}

func (r *RateTracker) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rates)
}

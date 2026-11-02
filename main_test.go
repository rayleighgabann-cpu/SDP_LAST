package main

import (
	"encoding/json"
	"math"
	"sync"
	"testing"
)

func fixtureBank() *Bank {
	client := &Client{
		Name:     "C",
		Accounts: []*Account{{ID: "A1", Balance: 1000, Rate: 0.10}},
		Cards:    []*Card{{ID: "CD1", Limit: 3000, Spent: 500}},
		Loans:    []*Loan{{ID: "L1", Principal: 5000, Rate: 0.12, Months: 12}},
		Deposits: []*Deposit{{ID: "D1", Amount: 2000, Rate: 0.05}},
	}
	return &Bank{Name: "B", Branches: []*Branch{{Name: "BR", Clients: []*Client{client}}}}
}

func closeTo(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

// ---- observer ----

func TestPublishReachesOnlyMatchingType(t *testing.T) {
	bus := NewEventBus()
	audit := NewAuditLog()
	bus.Subscribe(EvTransaction, audit)
	bus.Publish(NewEvent(EvTransaction, Transaction{ID: "T1", Amount: 1}))
	bus.Publish(NewEvent(EvRateChange, 0.05))
	if audit.Count() != 1 {
		t.Fatalf("expected 1 entry, got %d", audit.Count())
	}
}

func TestUnsubscribeAtRuntime(t *testing.T) {
	bus := NewEventBus()
	audit := NewAuditLog()
	bus.Subscribe(EvTransaction, audit)
	bus.Publish(NewEvent(EvTransaction, Transaction{ID: "T1"}))
	bus.Unsubscribe(EvTransaction, audit)
	bus.Publish(NewEvent(EvTransaction, Transaction{ID: "T2"}))
	if audit.Count() != 1 {
		t.Fatalf("expected 1 entry after unsubscribe, got %d", audit.Count())
	}
}

type oneShot struct {
	bus  Subject
	hits int
}

func (o *oneShot) Handle(Event) {
	o.hits++
	o.bus.Unsubscribe(EvTransaction, o) // leave from inside own handler
}

func TestUnsubscribeInsideOwnHandler(t *testing.T) {
	bus := NewEventBus()
	listener := &oneShot{bus: bus}
	bus.Subscribe(EvTransaction, listener)
	bus.Publish(NewEvent(EvTransaction, Transaction{ID: "T1"}))
	bus.Publish(NewEvent(EvTransaction, Transaction{ID: "T2"}))
	if listener.hits != 1 {
		t.Fatalf("expected 1 hit, got %d", listener.hits)
	}
}

func TestFraudDetectorRaisesAlert(t *testing.T) {
	bus := NewEventBus()
	audit := NewAuditLog()
	fraud := NewFraudDetector(5000, bus)
	bus.Subscribe(EvTransaction, fraud)
	bus.Subscribe(EvFraudAlert, audit)
	bus.Publish(NewEvent(EvTransaction, Transaction{ID: "T1", Amount: 6000}))
	bus.Publish(NewEvent(EvTransaction, Transaction{ID: "T2", Amount: 100}))
	if audit.Count() != 1 || fraud.FlaggedCount() != 1 {
		t.Fatalf("expected exactly one alert, audit=%d flagged=%d", audit.Count(), fraud.FlaggedCount())
	}
}

func TestAggregatorRepublishesChain(t *testing.T) {
	bus := NewEventBus()
	audit := NewAuditLog()
	agg := NewAggregatorListener(bus)
	bus.Subscribe(EvTransaction, agg)
	bus.Subscribe(EvAggregated, audit)
	bus.Publish(NewEvent(EvTransaction, Transaction{ID: "T1", Amount: 100}))
	bus.Publish(NewEvent(EvTransaction, Transaction{ID: "T2", Amount: 200}))
	if audit.Count() != 2 || !closeTo(agg.Sum(), 300) {
		t.Fatalf("chain broken: audit=%d sum=%f", audit.Count(), agg.Sum())
	}
}

func TestRateFeedSecondSubject(t *testing.T) {
	rates := NewEventBus()
	tracker := NewRateTracker()
	rates.Subscribe(EvRateChange, tracker)
	rates.Publish(NewEvent(EvRateChange, 0.05))
	if tracker.Count() != 1 {
		t.Fatalf("expected 1 rate, got %d", tracker.Count())
	}
}

func TestLimitMonitorTracksSpending(t *testing.T) {
	limits := NewLimitMonitor(600)
	limits.Handle(NewEvent(EvTransaction, Transaction{Client: "X", Amount: 300}))
	limits.Handle(NewEvent(EvTransaction, Transaction{Client: "X", Amount: 400}))
	if len(limits.OverLimit()) != 1 || limits.OverLimit()[0] != "X" {
		t.Fatalf("expected X over limit, got %v", limits.OverLimit())
	}
}

// ---- visitors, hand-calculated ----
// interest: 1000*0.10 + 5000*0.12*12/12 + 2000*0.05 = 100 + 600 + 100 = 800
// assets: 1000 + 2000 = 3000; liabilities: 5000 + 500 = 5500

func TestInterestHandCalculated(t *testing.T) {
	v := &InterestVisitor{}
	fixtureBank().Accept(v)
	if !closeTo(v.Result(), 800) {
		t.Fatalf("expected 800, got %f", v.Result())
	}
}

func TestBalanceHandCalculated(t *testing.T) {
	v := &BalanceVisitor{}
	fixtureBank().Accept(v)
	if !closeTo(v.Assets(), 3000) || !closeTo(v.Liabilities(), 5500) {
		t.Fatalf("expected 3000/5500, got %f/%f", v.Assets(), v.Liabilities())
	}
}

func TestHighValueReturnsElements(t *testing.T) {
	v := NewHighValueVisitor(1500)
	fixtureBank().Accept(v)
	matches := v.Result()
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if _, ok := matches[0].(*Loan); !ok {
		t.Fatalf("expected loan first, got %T", matches[0])
	}
	if _, ok := matches[1].(*Deposit); !ok {
		t.Fatalf("expected deposit second, got %T", matches[1])
	}
}

func TestStatementIsValidJSON(t *testing.T) {
	v := &StatementExporter{}
	fixtureBank().Accept(v)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(v.Result()), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, v.Result())
	}
	branches := parsed["branches"].([]any)
	clients := branches[0].(map[string]any)["clients"].([]any)
	products := clients[0].(map[string]any)["products"].([]any)
	if len(products) != 4 {
		t.Fatalf("expected 4 nested products, got %d", len(products))
	}
}

func TestFeeVisitorTouchesNoElements(t *testing.T) {
	v := &FeeVisitor{}
	fixtureBank().Accept(v)
	// 5 (account) + 2 (card) + 10 (loan) + 1 (deposit) = 18
	if !closeTo(v.Result(), 18) {
		t.Fatalf("expected 18, got %f", v.Result())
	}
}

// ---- integration ----

type reportSink struct{ got Report }

func (s *reportSink) Handle(e Event) {
	if r, ok := e.Payload.(Report); ok {
		s.got = r
	}
}

func TestInspectionIntegration(t *testing.T) {
	bus := NewEventBus()
	bank := fixtureBank()
	bank.Branches[0].Clients[0].Cards = append(bank.Branches[0].Clients[0].Cards,
		&Card{ID: "CD2", Limit: 100, Spent: 200})

	lookup := func(name string) *Client {
		for _, c := range bank.Branches[0].Clients {
			if c.Name == name {
				return c
			}
		}
		return nil
	}
	sink := &reportSink{}
	bus.Subscribe(EvFraudAlert, NewFraudInspectionHandler(bus, lookup))
	bus.Subscribe(EvInspection, sink)

	bus.Publish(NewEvent(EvFraudAlert, Transaction{ID: "T1", Client: "C", Amount: 9999}))
	if len(sink.got.Issues) != 1 || sink.got.Issues[0] != "CD2" {
		t.Fatalf("expected [CD2], got %v", sink.got.Issues)
	}
}

// ---- concurrency ----

func TestStress4x1000ExactCount(t *testing.T) {
	bus := NewEventBus()
	notifier := NewBalanceNotifier()
	bus.Subscribe(EvTransaction, notifier)

	const goroutines, perGoroutine = 4, 1000
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				bus.Publish(NewEvent(EvTransaction, Transaction{ID: "S", Amount: 1}))
			}
		}()
	}
	wg.Wait()

	if notifier.Count() != goroutines*perGoroutine {
		t.Fatalf("expected %d, got %d", goroutines*perGoroutine, notifier.Count())
	}
}

func TestGracefulShutdownCountsAllEvents(t *testing.T) {
	bus := NewEventBus()
	audit := NewAuditLog()
	bus.Subscribe(EvTransaction, audit)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 250; j++ {
				bus.Publish(NewEvent(EvTransaction, Transaction{ID: "G"}))
			}
		}()
	}
	wg.Wait()

	if audit.Count() != 1000 {
		t.Fatalf("lost events: expected 1000, got %d", audit.Count())
	}
}

func TestRegistryChurnDuringPublish(t *testing.T) {
	bus := NewEventBus()
	notifier := NewBalanceNotifier()
	bus.Subscribe(EvTransaction, notifier)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { // churn: join and leave while others publish
		defer wg.Done()
		dummy := NewAuditLog()
		for i := 0; i < 100; i++ {
			bus.Subscribe(EvTransaction, dummy)
			bus.Unsubscribe(EvTransaction, dummy)
		}
	}()
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				bus.Publish(NewEvent(EvTransaction, Transaction{ID: "C"}))
			}
		}()
	}
	wg.Wait()

	if notifier.Count() != 1000 {
		t.Fatalf("expected 1000, got %d", notifier.Count())
	}
}

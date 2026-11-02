package main

import (
	"fmt"
	"math/rand"
	"sync"
)

// producerCount is the configurable producer-thread count (ch. 13).
const producerCount = 4
const eventsPerProducer = 250

func sampleBank() *Bank {
	alice := &Client{
		Name:     "Alice",
		Accounts: []*Account{{ID: "ACC-1", Balance: 15000, Rate: 0.03}},
		Cards:    []*Card{{ID: "CARD-1", Limit: 5000, Spent: 1200}},
		Loans:    []*Loan{{ID: "LOAN-1", Principal: 20000, Rate: 0.07, Months: 24}},
		Deposits: []*Deposit{{ID: "DEP-1", Amount: 8000, Rate: 0.05}},
	}
	bob := &Client{
		Name:     "Bob",
		Accounts: []*Account{{ID: "ACC-2", Balance: 4200, Rate: 0.02}},
		Loans:    []*Loan{{ID: "LOAN-2", Principal: 50000, Rate: 0.09, Months: 60}},
	}
	carol := &Client{
		Name:     "Carol",
		Cards:    []*Card{{ID: "CARD-2", Limit: 10000, Spent: 11000}},
		Deposits: []*Deposit{{ID: "DEP-2", Amount: 30000, Rate: 0.04}},
	}
	return &Bank{
		Name: "CentralBank",
		Branches: []*Branch{
			{Name: "Downtown", Clients: []*Client{alice, bob}},
			{Name: "Uptown", Clients: []*Client{carol}},
		},
	}
}

func clientLookup(b *Bank) func(string) *Client {
	byName := map[string]*Client{}
	for _, branch := range b.Branches {
		for _, client := range branch.Clients {
			byName[client.Name] = client
		}
	}
	return func(name string) *Client { return byName[name] }
}

func main() {
	txFeed := NewEventBus()   // subject 1: transaction stream
	rateFeed := NewEventBus() // subject 2: rate stream

	audit := NewAuditLog()
	fraud := NewFraudDetector(9000, txFeed)
	notifier := NewBalanceNotifier()
	limits := NewLimitMonitor(50000)
	aggregator := NewAggregatorListener(txFeed) // observer + subject (chain)
	rateTracker := NewRateTracker()

	txFeed.Subscribe(EvTransaction, fraud)
	txFeed.Subscribe(EvTransaction, notifier)
	txFeed.Subscribe(EvTransaction, limits)
	txFeed.Subscribe(EvTransaction, aggregator)
	txFeed.Subscribe(EvTransaction, audit)
	txFeed.Subscribe(EvAggregated, audit)
	txFeed.Subscribe(EvFraudAlert, audit)
	rateFeed.Subscribe(EvRateChange, rateTracker)

	bank := sampleBank()
	txFeed.Subscribe(EvFraudAlert, NewFraudInspectionHandler(txFeed, clientLookup(bank)))
	txFeed.Subscribe(EvInspection, audit)

	// ---- producers (goroutines = Go equivalent of ExecutorService tasks)
	var wg sync.WaitGroup
	names := []string{"Alice", "Bob", "Carol"}
	for i := 0; i < producerCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerProducer; j++ {
				txFeed.Publish(NewEvent(EvTransaction, Transaction{
					ID:     fmt.Sprintf("TX-%d-%d", id, j),
					Client: names[rand.Intn(len(names))],
					Amount: rand.Float64() * 12000,
				}))
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			rateFeed.Publish(NewEvent(EvRateChange, 0.03+rand.Float64()*0.04))
		}
	}()
	wg.Wait() // graceful shutdown:

	// ---- visitors over the stable tree
	interest := &InterestVisitor{}
	bank.Accept(interest)

	balances := &BalanceVisitor{}
	bank.Accept(balances)

	highValue := NewHighValueVisitor(10000)
	bank.Accept(highValue)

	statement := &StatementExporter{}
	bank.Accept(statement)

	fmt.Printf("interest income: %.2f\n", interest.Result())
	fmt.Printf("assets: %.2f, liabilities: %.2f\n", balances.Assets(), balances.Liabilities())
	fmt.Printf("high-value products: %d\n", len(highValue.Result()))
	fmt.Printf("fraud alerts: %d, rates seen: %d\n", fraud.FlaggedCount(), rateTracker.Count())
	fmt.Println(statement.Result())
	fmt.Printf("audit entries: %d\n", audit.Count())
}

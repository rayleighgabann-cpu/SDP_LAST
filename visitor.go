package main

import (
	"fmt"
	"strings"
)

// Visitor declares one method per concrete element type. An element's Accept
// calls the matching method, which is the second dispatch of double dispatch.
type Visitor interface {
	VisitBank(*Bank)
	VisitBranch(*Branch)
	VisitClient(*Client)
	VisitAccount(*Account)
	VisitCard(*Card)
	VisitLoan(*Loan)
	VisitDeposit(*Deposit)
}

// visitAll lets the visitor drive the traversal: containers only hand over
// their children, so all walking logic stays in one place.
func visitAll(e Element, v Visitor) {
	for _, child := range e.Children() {
		child.Accept(v)
	}
}

// InterestVisitor is the numeric visitor: yearly interest income.
type InterestVisitor struct{ total float64 }

func (v *InterestVisitor) VisitBank(b *Bank)       { visitAll(b, v) }
func (v *InterestVisitor) VisitBranch(b *Branch)   { visitAll(b, v) }
func (v *InterestVisitor) VisitClient(c *Client)   { visitAll(c, v) }
func (v *InterestVisitor) VisitAccount(a *Account) { v.total += a.Balance * a.Rate }
func (v *InterestVisitor) VisitCard(*Card)         {}
func (v *InterestVisitor) VisitLoan(l *Loan) {
	v.total += l.Principal * l.Rate * float64(l.Months) / 12
}
func (v *InterestVisitor) VisitDeposit(d *Deposit) { v.total += d.Amount * d.Rate }
func (v *InterestVisitor) Result() float64         { return v.total }

// BalanceVisitor accumulates state across the whole tree.
type BalanceVisitor struct {
	assets      float64
	liabilities float64
}

func (v *BalanceVisitor) VisitBank(b *Bank)       { visitAll(b, v) }
func (v *BalanceVisitor) VisitBranch(b *Branch)   { visitAll(b, v) }
func (v *BalanceVisitor) VisitClient(c *Client)   { visitAll(c, v) }
func (v *BalanceVisitor) VisitAccount(a *Account) { v.assets += a.Balance }
func (v *BalanceVisitor) VisitCard(c *Card)       { v.liabilities += c.Spent }
func (v *BalanceVisitor) VisitLoan(l *Loan)       { v.liabilities += l.Principal }
func (v *BalanceVisitor) VisitDeposit(d *Deposit) { v.assets += d.Amount }
func (v *BalanceVisitor) Assets() float64         { return v.assets }
func (v *BalanceVisitor) Liabilities() float64    { return v.liabilities }

// HighValueVisitor is the filter visitor: it returns a list of elements.
type HighValueVisitor struct {
	threshold float64
	matches   []Element
}

func NewHighValueVisitor(threshold float64) *HighValueVisitor {
	return &HighValueVisitor{threshold: threshold}
}

func (v *HighValueVisitor) VisitBank(b *Bank)     { visitAll(b, v) }
func (v *HighValueVisitor) VisitBranch(b *Branch) { visitAll(b, v) }
func (v *HighValueVisitor) VisitClient(c *Client) { visitAll(c, v) }
func (v *HighValueVisitor) VisitAccount(a *Account) {
	if a.Balance >= v.threshold {
		v.matches = append(v.matches, a)
	}
}
func (v *HighValueVisitor) VisitCard(*Card) {}
func (v *HighValueVisitor) VisitLoan(l *Loan) {
	if l.Principal >= v.threshold {
		v.matches = append(v.matches, l)
	}
}
func (v *HighValueVisitor) VisitDeposit(d *Deposit) {
	if d.Amount >= v.threshold {
		v.matches = append(v.matches, d)
	}
}
func (v *HighValueVisitor) Result() []Element { return v.matches }

type StatementExporter struct {
	depth int
	sb    strings.Builder
}

func (v *StatementExporter) VisitBank(b *Bank) {
	v.printf(`{"bank":%q,"branches":[`, b.Name)
	v.depth++
	for i, branch := range b.Branches {
		v.sep(i)
		branch.Accept(v)
	}
	v.depth--
	v.printf("\n%s]}", v.pad())
}

func (v *StatementExporter) VisitBranch(b *Branch) {
	v.printf(`{"branch":%q,"clients":[`, b.Name)
	v.depth++
	for i, client := range b.Clients {
		v.sep(i)
		client.Accept(v)
	}
	v.depth--
	v.printf("\n%s]}", v.pad())
}

func (v *StatementExporter) VisitClient(c *Client) {
	v.printf(`{"client":%q,"products":[`, c.Name)
	v.depth++
	for i, product := range c.Children() {
		v.sep(i)
		product.Accept(v)
	}
	v.depth--
	v.printf("\n%s]}", v.pad())
}

func (v *StatementExporter) VisitAccount(a *Account) {
	v.printf(`{"type":"account","id":%q,"balance":%.2f}`, a.ID, a.Balance)
}
func (v *StatementExporter) VisitCard(c *Card) {
	v.printf(`{"type":"card","id":%q,"limit":%.2f,"spent":%.2f}`, c.ID, c.Limit, c.Spent)
}
func (v *StatementExporter) VisitLoan(l *Loan) {
	v.printf(`{"type":"loan","id":%q,"principal":%.2f,"rate":%.4f}`, l.ID, l.Principal, l.Rate)
}
func (v *StatementExporter) VisitDeposit(d *Deposit) {
	v.printf(`{"type":"deposit","id":%q,"amount":%.2f}`, d.ID, d.Amount)
}

func (v *StatementExporter) Result() string { return v.sb.String() }

func (v *StatementExporter) pad() string { return strings.Repeat("  ", v.depth) }
func (v *StatementExporter) sep(i int) {
	if i > 0 {
		v.sb.WriteString(",")
	}
	v.sb.WriteString("\n" + v.pad())
}
func (v *StatementExporter) printf(format string, args ...any) {
	fmt.Fprintf(&v.sb, format, args...)
}

// ---- integration: observer event -> visitor on one subtree -> new event ----

// Report is the payload of the inspection event.
type Report struct {
	Client string
	Issues []string
}

// InspectionVisitor checks a subtree for anomalies.
type InspectionVisitor struct{ issues []string }

func (v *InspectionVisitor) VisitBank(b *Bank)     { visitAll(b, v) }
func (v *InspectionVisitor) VisitBranch(b *Branch) { visitAll(b, v) }
func (v *InspectionVisitor) VisitClient(c *Client) { visitAll(c, v) }
func (v *InspectionVisitor) VisitAccount(a *Account) {
	if a.Balance < 0 {
		v.issues = append(v.issues, a.ID)
	}
}
func (v *InspectionVisitor) VisitCard(c *Card) {
	if c.Spent > c.Limit {
		v.issues = append(v.issues, c.ID)
	}
}
func (v *InspectionVisitor) VisitLoan(*Loan)       {}
func (v *InspectionVisitor) VisitDeposit(*Deposit) {}
func (v *InspectionVisitor) Result() []string      { return v.issues }

// FraudInspectionHandler is the integration link: on a fraud alert it runs
// InspectionVisitor on the affected client only and publishes the report.
type FraudInspectionHandler struct {
	bus     Subject
	clients func(name string) *Client
}

func NewFraudInspectionHandler(bus Subject, clients func(string) *Client) *FraudInspectionHandler {
	return &FraudInspectionHandler{bus: bus, clients: clients}
}

func (h *FraudInspectionHandler) Handle(e Event) {
	tx, ok := e.Payload.(Transaction)
	if !ok {
		return
	}
	client := h.clients(tx.Client)
	if client == nil {
		return
	}
	inspection := &InspectionVisitor{}
	client.Accept(inspection)
	h.bus.Publish(NewEvent(EvInspection, Report{Client: tx.Client, Issues: inspection.Result()}))
}

package main

// Element is a node of the stable composite tree. Elements hold data only;
// every operation on the tree lives in a visitor.
type Element interface {
	Accept(Visitor)
	Children() []Element
}

// elements converts a typed slice to []Element without repeating the loop.
func elements[T Element](items []T) []Element {
	out := make([]Element, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

// ---- containers (2 types below the root) ----

type Bank struct {
	Name     string
	Branches []*Branch
}

func (b *Bank) Accept(v Visitor)    { v.VisitBank(b) }
func (b *Bank) Children() []Element { return elements(b.Branches) }

type Branch struct {
	Name    string
	Clients []*Client
}

func (b *Branch) Accept(v Visitor)    { v.VisitBranch(b) }
func (b *Branch) Children() []Element { return elements(b.Clients) }

type Client struct {
	Name     string
	Accounts []*Account
	Cards    []*Card
	Loans    []*Loan
	Deposits []*Deposit
}

func (c *Client) Accept(v Visitor) { v.VisitClient(c) }
func (c *Client) Children() []Element {
	out := make([]Element, 0, len(c.Accounts)+len(c.Cards)+len(c.Loans)+len(c.Deposits))
	out = append(out, elements(c.Accounts)...)
	out = append(out, elements(c.Cards)...)
	out = append(out, elements(c.Loans)...)
	out = append(out, elements(c.Deposits)...)
	return out
}

// ---- leaves (4 types) ----

type Account struct {
	ID      string
	Balance float64
	Rate    float64
}

func (a *Account) Accept(v Visitor)    { v.VisitAccount(a) }
func (a *Account) Children() []Element { return nil }

type Card struct {
	ID    string
	Limit float64
	Spent float64
}

func (c *Card) Accept(v Visitor)    { v.VisitCard(c) }
func (c *Card) Children() []Element { return nil }

type Loan struct {
	ID        string
	Principal float64
	Rate      float64
	Months    int
}

func (l *Loan) Accept(v Visitor)    { v.VisitLoan(l) }
func (l *Loan) Children() []Element { return nil }

type Deposit struct {
	ID     string
	Amount float64
	Rate   float64
}

func (d *Deposit) Accept(v Visitor)    { v.VisitDeposit(d) }
func (d *Deposit) Children() []Element { return nil }

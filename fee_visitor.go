package main

// FeeVisitor is the trade-off proof: a brand-new operation added without
// touching a single element class (see README, trade-off commits).
type FeeVisitor struct{ total float64 }

func (v *FeeVisitor) VisitBank(b *Bank)     { visitAll(b, v) }
func (v *FeeVisitor) VisitBranch(b *Branch) { visitAll(b, v) }
func (v *FeeVisitor) VisitClient(c *Client) { visitAll(c, v) }
func (v *FeeVisitor) VisitAccount(*Account) { v.total += 5 }
func (v *FeeVisitor) VisitCard(*Card)       { v.total += 2 }
func (v *FeeVisitor) VisitLoan(*Loan)       { v.total += 10 }
func (v *FeeVisitor) VisitDeposit(*Deposit) { v.total += 1 }
func (v *FeeVisitor) Result() float64       { return v.total }

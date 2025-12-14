package models

import "time"

type Expense struct {
	ID          int
	Description string
	Amount      float64
	PaidBy      string
	CreatedAt   time.Time
}

type Split struct {
	ID        int
	ExpenseID int
	UserName  string
	Amount    float64
}

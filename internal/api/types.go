package api

// Типы повторяют то, что отдаёт приложение (lib/utils/payment.utils.ts и
// схемы в lib/db/schemas). Поля, которых CLI не показывает, опущены.

type User struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	DefaultCurrency string `json:"defaultCurrency"`
}

type Tag struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Color    *string `json:"color"`
	ParentID *string `json:"parentId"`
}

type Counterparty struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Type string  `json:"type"`
	Icon *string `json:"icon"`
}

type Payment struct {
	ID                   string         `json:"id"`
	Status               string         `json:"status"`
	Amount               string         `json:"amount"`
	Currency             string         `json:"currency"`
	AmountInBaseCurrency string         `json:"amountInBaseCurrency"`
	BaseCurrency         string         `json:"baseCurrency"`
	Date                 string         `json:"date"`
	Description          *string        `json:"description"`
	Details              *string        `json:"details"`
	Tags                 []Tag          `json:"tags"`
	Counterparties       []Counterparty `json:"counterparties"`
}

type Template struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Type            *string  `json:"type"`
	Status          *string  `json:"status"`
	Amount          *string  `json:"amount"`
	Currency        *string  `json:"currency"`
	Description     *string  `json:"description"`
	TagIDs          []string `json:"tagIds"`
	CounterpartyIDs []string `json:"counterpartyIds"`
}

// CreatePayment повторяет createPaymentSchema. Указатели там, где сервер
// различает «не передано» и «пусто».
type CreatePayment struct {
	Status          string   `json:"status,omitempty"`
	Amount          string   `json:"amount"`
	Currency        string   `json:"currency"`
	Date            string   `json:"date"`
	Description     *string  `json:"description,omitempty"`
	TagIDs          []string `json:"tagIds,omitempty"`
	CounterpartyIDs []string `json:"counterpartyIds,omitempty"`
}

type CreateTag struct {
	Name  string  `json:"name"`
	Color *string `json:"color,omitempty"`
}

type CreateCounterparty struct {
	Name string  `json:"name"`
	Type string  `json:"type"`
	Icon *string `json:"icon,omitempty"`
}

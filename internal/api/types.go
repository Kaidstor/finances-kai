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

// UpdatePayment повторяет updatePaymentSchema: сервер меняет только те поля,
// что пришли, поэтому здесь всё указатели с omitempty.
type UpdatePayment struct {
	Status          *string   `json:"status,omitempty"`
	Amount          *string   `json:"amount,omitempty"`
	Currency        *string   `json:"currency,omitempty"`
	Date            *string   `json:"date,omitempty"`
	Description     *string   `json:"description,omitempty"`
	TagIDs          *[]string `json:"tagIds,omitempty"`
	CounterpartyIDs *[]string `json:"counterpartyIds,omitempty"`
}

type CreateTemplate struct {
	Name            string   `json:"name"`
	Type            *string  `json:"type,omitempty"`
	Status          string   `json:"status,omitempty"`
	Amount          *string  `json:"amount,omitempty"`
	Currency        *string  `json:"currency,omitempty"`
	Description     *string  `json:"description,omitempty"`
	TagIDs          []string `json:"tagIds,omitempty"`
	CounterpartyIDs []string `json:"counterpartyIds,omitempty"`
}

type PaymentFile struct {
	ID           string `json:"id"`
	OriginalName string `json:"originalName"`
	MimeType     string `json:"mimeType"`
	Size         int64  `json:"size"`
	CreatedAt    string `json:"createdAt"`
}

type ForecastPayment struct {
	Date                 string `json:"date"`
	SubscriptionID       string `json:"subscriptionId"`
	SubscriptionName     string `json:"subscriptionName"`
	Amount               string `json:"amount"`
	AmountInBaseCurrency string `json:"amountInBaseCurrency"`
	Currency             string `json:"currency"`
}

type ForecastMonth struct {
	Month     string            `json:"month"`
	MonthName string            `json:"monthName"`
	Total     float64           `json:"total"`
	Payments  []ForecastPayment `json:"payments"`
}

type ForecastSubscription struct {
	SubscriptionID   string  `json:"subscriptionId"`
	SubscriptionName string  `json:"subscriptionName"`
	Total            float64 `json:"total"`
	Count            int     `json:"count"`
}

type Forecast struct {
	BaseCurrency string `json:"baseCurrency"`
	Summary      struct {
		Total1Month  float64 `json:"total1Month"`
		Total3Months float64 `json:"total3Months"`
		Total6Months float64 `json:"total6Months"`
		TotalYear    float64 `json:"totalYear"`
	} `json:"summary"`
	ByMonth        []ForecastMonth        `json:"byMonth"`
	BySubscription []ForecastSubscription `json:"bySubscription"`
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

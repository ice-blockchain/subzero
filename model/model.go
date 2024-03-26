package model

type (
	Event struct {
		Tags      [][]string `json:"tags" db:"tags"`
		Content   string     `json:"content" db:"content"`
		Signature string     `json:"sig" db:"sig"`
		ID        string     `json:"id" db:"id"`
		PublicKey string     `json:"pubkey" db:"pubkey"`
		CreatedAt uint32     `json:"created_at" db:"created_at"`
		Kind      uint16     `json:"kind" db:"kind"`
	}
	Subscription struct {
		Filters []*SubscriptionFilter
	}
	SubscriptionFilter struct {
		E          *[]string `json:"#e"`
		P          *[]string `json:"#p"`
		A          *[]string `json:"#a"`
		D          *[]string `json:"#d"`
		G          *[]string `json:"#g"`
		I          *[]string `json:"#i"`
		K          *[]string `json:"#k"`
		L          *[]string `json:"#l"`
		LUppercase *[]string `json:"#L"`
		M          *[]string `json:"#m"`
		Q          *[]string `json:"#q"`
		R          *[]string `json:"#r"`
		T          *[]string `json:"#t"`
		IDs        *[]string `json:"ids"`
		Authors    *[]string `json:"authors"`
		Search     *string   `json:"search"`
		Kinds      *[]uint16 `json:"kinds"`
		Since      *uint32   `json:"since"`
		Until      *uint32   `json:"until"`
		Limit      *uint16   `json:"limit"`
	}
)

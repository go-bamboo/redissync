package reader

import (
	"context"
	"github.com/go-bamboo/redissync/entry"
)

type Statusable interface {
	Status() interface{}
	StatusString() string
	StatusConsistent() bool
}

type Reader interface {
	Statusable
	StartRead(ctx context.Context) chan *entry.Entry
}

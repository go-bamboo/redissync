package writer

import (
	"github.com/go-bamboo/redissync/entry"
)

type Statusable interface {
	Status() interface{}
	StatusString() string
	StatusConsistent() bool
}

type Writer interface {
	Statusable
	Write(entry *entry.Entry)
	Close()
}

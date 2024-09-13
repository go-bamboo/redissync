package sync

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-bamboo/redissync/entry"
	"github.com/go-bamboo/redissync/log"
	"github.com/go-bamboo/redissync/reader"
	"github.com/go-bamboo/redissync/utils"
	"github.com/mcuadros/go-defaults"
)

var _ reader.Reader = (*syncClusterReader)(nil)

type syncClusterReader struct {
	readers  []reader.Reader
	statusId int
}

func NewSyncClusterReader(ctx context.Context, opts ...SyncReaderOption) reader.Reader {
	opt := new(SyncReaderOptions)
	defaults.SetDefaults(opt)
	for _, o := range opts {
		o(opt)
	}
	addresses, _ := utils.GetRedisClusterNodes(ctx, opt.address, opt.username, opt.password, opt.tls, opt.preferReplica)
	log.Debugf("get redis cluster nodes:")
	for _, address := range addresses {
		log.Debugf("%s", address)
	}
	rd := &syncClusterReader{}
	for _, address := range addresses {
		theOpts := *opt
		theOpts.address = address
		rd.readers = append(rd.readers, newSyncStandaloneReader(ctx, &theOpts))
	}
	return rd
}

func (rd *syncClusterReader) StartRead(ctx context.Context) chan *entry.Entry {
	ch := make(chan *entry.Entry, 1024)
	var wg sync.WaitGroup
	for _, r := range rd.readers {
		wg.Add(1)
		go func(r reader.Reader) {
			defer wg.Done()
			for e := range r.StartRead(ctx) {
				ch <- e
			}
		}(r)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	return ch
}

func (rd *syncClusterReader) Status() interface{} {
	stat := make([]interface{}, 0)
	for _, r := range rd.readers {
		stat = append(stat, r.Status())
	}
	return stat
}

func (rd *syncClusterReader) StatusString() string {
	rd.statusId += 1
	rd.statusId %= len(rd.readers)
	return fmt.Sprintf("src-%d, %s", rd.statusId, rd.readers[rd.statusId].StatusString())
}

func (rd *syncClusterReader) StatusConsistent() bool {
	for _, r := range rd.readers {
		if !r.StatusConsistent() {
			return false
		}
	}
	return true
}

package reader

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-bamboo/redissync/entry"
	"github.com/go-bamboo/redissync/utils"
)

type scanClusterReader struct {
	readers  []Reader
	statusId int
}

func NewScanClusterReader(ctx context.Context, opts *ScanReaderOptions) Reader {
	addresses, _ := utils.GetRedisClusterNodes(ctx, opts.Address, opts.Username, opts.Password, opts.Tls, opts.PreferReplica)

	rd := &scanClusterReader{}
	for _, address := range addresses {
		theOpts := *opts
		theOpts.Address = address
		rd.readers = append(rd.readers, NewScanStandaloneReader(ctx, &theOpts))
	}
	return rd
}

func (rd *scanClusterReader) StartRead(ctx context.Context) chan *entry.Entry {
	ch := make(chan *entry.Entry, 1024)
	var wg sync.WaitGroup
	for _, r := range rd.readers {
		wg.Add(1)
		go func(r Reader) {
			for e := range r.StartRead(ctx) {
				ch <- e
			}
			wg.Done()
		}(r)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	return ch
}

func (rd *scanClusterReader) Status() interface{} {
	stat := make([]interface{}, 0)
	for _, r := range rd.readers {
		stat = append(stat, r.Status())
	}
	return stat
}

func (rd *scanClusterReader) StatusString() string {
	rd.statusId += 1
	rd.statusId %= len(rd.readers)
	return fmt.Sprintf("src-%d, %s", rd.statusId, rd.readers[rd.statusId].StatusString())
}

func (rd *scanClusterReader) StatusConsistent() bool {
	for _, r := range rd.readers {
		if !r.StatusConsistent() {
			return false
		}
	}
	return true
}

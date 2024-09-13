package sync

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-bamboo/redissync/client"
	"github.com/go-bamboo/redissync/entry"
	"github.com/go-bamboo/redissync/log"
	"github.com/go-bamboo/redissync/rdb"
	"github.com/go-bamboo/redissync/reader"
	"github.com/go-bamboo/redissync/utils"
	rotate "github.com/go-bamboo/redissync/utils/file_rotate"
	defaults "github.com/mcuadros/go-defaults"
)

type State string

const (
	kHandShake  State = "hand shaking"
	kWaitBgsave State = "waiting bgsave"
	kReceiveRdb State = "receiving rdb"
	kSyncRdb    State = "syncing rdb"
	kSyncAof    State = "syncing aof"
)

var _ reader.Reader = (*syncStandaloneReader)(nil)

type syncStandaloneReader struct {
	ctx    context.Context
	opts   *SyncReaderOptions
	client *client.Redis

	ch   chan *entry.Entry
	DbId int

	rd *bufio.Reader

	stat struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		Dir     string `json:"dir"`

		// status
		Status State `json:"status"`

		// rdb info
		RdbFileSizeBytes int64  `json:"rdb_file_size_bytes"` // bytes of the rdb file
		RdbFileSizeHuman string `json:"rdb_file_size_human"`
		RdbReceivedBytes int64  `json:"rdb_received_bytes"` // bytes of RDB received from master
		RdbReceivedHuman string `json:"rdb_received_human"`
		RdbSentBytes     int64  `json:"rdb_sent_bytes"` // bytes of RDB sent to chan
		RdbSentHuman     string `json:"rdb_sent_human"`

		// aof info
		AofReceivedOffset int64  `json:"aof_received_offset"` // offset of AOF received from master
		AofSentOffset     int64  `json:"aof_sent_offset"`     // offset of AOF sent to chan
		AofReceivedBytes  int64  `json:"aof_received_bytes"`  // bytes of AOF received from master
		AofReceivedHuman  string `json:"aof_received_human"`
	}
}

func NewSyncStandaloneReader(ctx context.Context, opts ...SyncReaderOption) reader.Reader {
	opt := new(SyncReaderOptions)
	defaults.SetDefaults(opt)
	for _, o := range opts {
		o(opt)
	}
	return newSyncStandaloneReader(ctx, opt)
}

func newSyncStandaloneReader(ctx context.Context, opt *SyncReaderOptions) reader.Reader {
	r := new(syncStandaloneReader)
	r.opts = opt
	r.client = client.NewRedisClient(ctx, opt.address, opt.username, opt.password, opt.tls, opt.preferReplica)
	r.rd = r.client.BufioReader()
	r.stat.Name = "reader_" + strings.Replace(opt.address, ":", "_", -1)
	r.stat.Address = opt.address
	r.stat.Status = kHandShake
	r.stat.Dir = utils.GetAbsPath(r.stat.Name)
	utils.CreateEmptyDir(r.stat.Dir)
	return r
}

func (r *syncStandaloneReader) StartRead(ctx context.Context) chan *entry.Entry {
	r.ctx = ctx
	r.ch = make(chan *entry.Entry, 1024)
	go func() {
		r.sendReplconfListenPort()
		r.sendPSync()
		rdbFilePath := r.receiveRDB()
		startOffset := r.stat.AofReceivedOffset
		go r.sendReplconfAck() // start sent replconf ack
		go r.receiveAOF(r.rd)
		if r.opts.syncRdb {
			r.sendRDB(rdbFilePath)
		}
		if r.opts.syncAof {
			r.stat.Status = kSyncAof
			r.sendAOF(startOffset)
		}
		close(r.ch)
	}()

	return r.ch
}

func (r *syncStandaloneReader) sendReplconfListenPort() {
	// use status_port as redis-shake port
	argv := []interface{}{"replconf", "listening-port", strconv.Itoa(r.opts.statusPort)}
	r.client.Send(argv...)
	_, err := r.client.Receive()
	if err != nil {
		log.Warnf("[%s] send replconf command to redis server failed. error=[%v]", r.stat.Name, err)
	}
}

func (r *syncStandaloneReader) sendPSync() {
	if r.opts.tryDiskless {
		argv := []interface{}{"REPLCONF", "CAPA", "EOF"}
		reply := r.client.DoWithStringReply(argv...)
		if reply != "OK" {
			log.Warnf("[%s] send replconf capa eof to redis server failed. reply=[%v]", r.stat.Name, reply)
		}
	}
	// send PSync
	argv := []interface{}{"PSYNC", "?", "-1"}
	if r.opts.AwsPSync != "" {
		argv = []interface{}{r.opts.GetPSyncCommand(r.stat.Address), "?", "-1"}
	}
	r.client.Send(argv...)

	// format: \n\n\n+<reply>\r\n
	for {
		select {
		case <-r.ctx.Done():
			close(r.ch)
			runtime.Goexit() // stop goroutine
		default:
		}
		bytes, err := r.rd.Peek(1)
		if err != nil {
			log.Panicf(err.Error())
		}
		if bytes[0] != '\n' {
			break
		}
		r.rd.ReadByte()
	}
	reply := r.client.ReceiveString()
	masterOffset, err := strconv.Atoi(strings.Split(reply, " ")[2])
	if err != nil {
		log.Panicf(err.Error())
	}
	r.stat.AofReceivedOffset = int64(masterOffset)
}

func (r *syncStandaloneReader) receiveRDB() string {
	log.Debugf("[%s] source db is doing bgsave.", r.stat.Name)
	r.stat.Status = kWaitBgsave
	timeStart := time.Now()
	// format: \n\n\n$<length>\r\n<rdb>
	// if source support repl-diskless-sync: \n\n\n$EOF:<40 characters EOF marker>\r\nstream data<EOF marker>
	for {
		b, err := r.rd.ReadByte()
		if err != nil {
			log.Panicf(err.Error())
		}
		if b == '\n' { // heartbeat
			continue
		}
		if b != '$' {
			log.Panicf("[%s] invalid rdb format. b=[%s]", r.stat.Name, string(b))
		}
		break
	}
	log.Debugf("[%s] source db bgsave finished. timeUsed=[%.2f]s", r.stat.Name, time.Since(timeStart).Seconds())
	marker, err := r.rd.ReadString('\n')
	if err != nil {
		log.Panicf(err.Error())
	}
	marker = strings.TrimSpace(marker)

	// create rdb file
	rdbFilePath, err := filepath.Abs(r.stat.Name + "/dump.rdb")
	if err != nil {
		log.Panicf(err.Error())
	}
	timeStart = time.Now()
	log.Debugf("[%s] start receiving RDB. path=[%s]", r.stat.Name, rdbFilePath)
	rdbFileHandle, err := os.OpenFile(rdbFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		log.Panicf(err.Error())
	}

	// receive rdb
	r.stat.Status = kReceiveRdb
	if strings.HasPrefix(marker, "EOF") {
		log.Infof("[%s] source db supoort diskless sync capability.", r.stat.Name)
		r.receiveRDBWithDiskless(marker, rdbFileHandle)
	} else {
		r.receiveRDBWithoutDiskless(marker, rdbFileHandle)
	}
	err = rdbFileHandle.Close()
	if err != nil {
		log.Panicf(err.Error())
	}
	log.Debugf("[%s] save RDB finished. timeUsed=[%.2f]s", r.stat.Name, time.Since(timeStart).Seconds())
	return rdbFilePath
}

func (r *syncStandaloneReader) receiveRDBWithDiskless(marker string, wt io.Writer) {
	const bufSize int64 = 32 * 1024 * 1024 // 32MB
	buf := make([]byte, bufSize)

	marker = strings.Split(marker, ":")[1]
	if len(marker) != 40 {
		log.Panicf("[%s] invalid len of EOF marker. value=[%s]", r.stat.Name, marker)
	}
	log.Infof("meet EOF begin marker: %s", marker)
	bMarker := []byte(marker)
	goon := true
	for goon {
		n, err := r.rd.Read(buf[:bufSize])
		if err != nil {
			log.Panicf(err.Error())
		}
		buffer := buf[:n]
		if bytes.Contains(buffer, bMarker) {
			log.Infof("meet EOF end marker.")
			// replace it
			fi := bytes.Index(buffer, bMarker)
			if len(buffer[fi+40:]) > 0 {
				log.Warnf("data after end marker will be discarded: %s", string(buffer[fi+40:]))
			}
			buffer = buffer[:fi]

			goon = false
		}

		_, err = wt.Write(buffer)
		if err != nil {
			log.Panicf(err.Error())
		}

		r.stat.RdbFileSizeBytes += int64(n)
		r.stat.RdbFileSizeHuman = humanize.IBytes(uint64(r.stat.RdbFileSizeBytes))
		r.stat.RdbReceivedBytes += int64(n)
		r.stat.RdbReceivedHuman = humanize.IBytes(uint64(r.stat.RdbReceivedBytes))
	}
}

func (r *syncStandaloneReader) receiveRDBWithoutDiskless(marker string, wt io.Writer) {
	length, err := strconv.ParseInt(marker, 10, 64)
	if err != nil {
		log.Panicf(err.Error())
	}
	log.Debugf("[%s] rdb file size: [%v]", r.stat.Name, humanize.IBytes(uint64(length)))
	r.stat.RdbFileSizeBytes = length
	r.stat.RdbFileSizeHuman = humanize.IBytes(uint64(length))

	remainder := length
	const bufSize int64 = 32 * 1024 * 1024 // 32MB
	buf := make([]byte, bufSize)
	for remainder != 0 {
		readOnce := bufSize
		if remainder < readOnce {
			readOnce = remainder
		}
		n, err := r.rd.Read(buf[:readOnce])
		if err != nil {
			log.Panicf(err.Error())
		}
		remainder -= int64(n)
		_, err = wt.Write(buf[:n])
		if err != nil {
			log.Panicf(err.Error())
		}

		r.stat.RdbReceivedBytes += int64(n)
		r.stat.RdbReceivedHuman = humanize.IBytes(uint64(r.stat.RdbReceivedBytes))
	}
}

func (r *syncStandaloneReader) receiveAOF(rd io.Reader) {
	log.Debugf("[%s] start receiving aof data, and save to file", r.stat.Name)
	aofWriter := rotate.NewAOFWriter(r.stat.Name, r.stat.Dir, r.stat.AofReceivedOffset)
	defer aofWriter.Close()
	buf := make([]byte, 16*1024) // 16KB is enough for writing file
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
			n, err := rd.Read(buf)
			if err != nil {
				log.Panicf(err.Error())
			}
			r.stat.AofReceivedBytes += int64(n)
			r.stat.AofReceivedHuman = humanize.IBytes(uint64(r.stat.AofReceivedBytes))
			aofWriter.Write(buf[:n])
			r.stat.AofReceivedOffset += int64(n)
		}
	}
}

func (r *syncStandaloneReader) sendRDB(rdbFilePath string) {
	// start parse rdb
	log.Debugf("[%s] start sending RDB to target", r.stat.Name)
	r.stat.Status = kSyncRdb
	updateFunc := func(offset int64) {
		r.stat.RdbSentBytes = offset
		r.stat.RdbSentHuman = humanize.IBytes(uint64(offset))
	}
	rdbLoader := rdb.NewLoader(r.stat.Name, updateFunc, rdbFilePath, r.ch)
	r.DbId = rdbLoader.ParseRDB(r.ctx)
	log.Debugf("[%s] send RDB finished", r.stat.Name)
	// delete file
	_ = os.Remove(rdbFilePath)
	log.Debugf("[%s] delete RDB file", r.stat.Name)
}

func (r *syncStandaloneReader) sendAOF(offset int64) {
	time.Sleep(1 * time.Second) // wait for receiveAOF create aof file
	aofReader := rotate.NewAOFReader(r.stat.Name, r.stat.Dir, offset)
	defer aofReader.Close()
	r.client.SetBufioReader(bufio.NewReader(aofReader))
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
			argv := client.ArrayString(r.client.Receive())
			r.stat.AofSentOffset = aofReader.Offset()
			// select
			if strings.EqualFold(argv[0], "select") {
				DbId, err := strconv.Atoi(argv[1])
				if err != nil {
					log.Panicf(err.Error())
				}
				r.DbId = DbId
				continue
			}
			// ping
			if strings.EqualFold(argv[0], "ping") {
				continue
			}
			// replconf @AWS
			if strings.EqualFold(argv[0], "replconf") {
				continue
			}
			// opinfo @Aliyun
			if strings.EqualFold(argv[0], "opinfo") {
				continue
			}
			// txn
			if strings.EqualFold(argv[0], "multi") || strings.EqualFold(argv[0], "exec") {
				continue
			}
			// sentinel
			if strings.EqualFold(argv[0], "publish") && strings.EqualFold(argv[1], "__sentinel__:hello") {
				continue
			}

			e := entry.NewEntry()
			e.Argv = argv
			e.DbId = r.DbId
			r.ch <- e
		}
	}
}

// sendReplconfAck send replconf ack to master to keep heartbeat between redis-shake and source redis.
func (r *syncStandaloneReader) sendReplconfAck() {
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	for range ticker.C {
		select {
		case <-r.ctx.Done():
			return
		default:
			if r.stat.AofReceivedOffset != 0 {
				r.client.Send("replconf", "ack", strconv.FormatInt(r.stat.AofReceivedOffset, 10))
			}
		}
	}
}

func (r *syncStandaloneReader) Status() interface{} {
	return r.stat
}

func (r *syncStandaloneReader) StatusString() string {
	if r.stat.Status == kSyncRdb {
		return fmt.Sprintf("%s, size=[%s/%s]", r.stat.Status, r.stat.RdbSentHuman, r.stat.RdbFileSizeHuman)
	}
	if r.stat.Status == kSyncAof {
		return fmt.Sprintf("%s, diff=[%v]", r.stat.Status, -r.stat.AofSentOffset+r.stat.AofReceivedOffset)
	}
	return string(r.stat.Status)
}

func (r *syncStandaloneReader) StatusConsistent() bool {
	return r.stat.AofReceivedOffset != 0 &&
		r.stat.AofReceivedOffset == r.stat.AofSentOffset &&
		len(r.ch) == 0
}

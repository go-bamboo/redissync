package sync

import (
	"github.com/go-bamboo/redissync/log"
	"strings"
)

// SyncReaderOption is an HTTP server option.
type SyncReaderOption func(*SyncReaderOptions)

// Cluster with server cluster.
func Cluster(cluster bool) SyncReaderOption {
	return func(s *SyncReaderOptions) {
		s.cluster = cluster
	}
}

// Address with server address.
func Address(address string) SyncReaderOption {
	return func(s *SyncReaderOptions) {
		s.address = address
	}
}

// Username with server address.
func Username(username string) SyncReaderOption {
	return func(s *SyncReaderOptions) {
		s.username = username
	}
}

// Password with server password.
func Password(password string) SyncReaderOption {
	return func(s *SyncReaderOptions) {
		s.password = password
	}
}

// Tls with server password.
func Tls(tls bool) SyncReaderOption {
	return func(s *SyncReaderOptions) {
		s.tls = tls
	}
}

type SyncReaderOptions struct {
	cluster       bool   `mapstructure:"cluster" default:"false"`
	address       string `mapstructure:"address" default:""`
	username      string `mapstructure:"username" default:""`
	password      string `mapstructure:"password" default:""`
	tls           bool   `mapstructure:"tls" default:"false"`
	SyncRdb       bool   `mapstructure:"sync_rdb" default:"true"`
	SyncAof       bool   `mapstructure:"sync_aof" default:"true"`
	PreferReplica bool   `mapstructure:"prefer_replica" default:"false"`
	TryDiskless   bool   `mapstructure:"try_diskless" default:"false"`

	// advanced
	StatusPort int    `mapstructure:"status_port" default:"0"`
	AwsPSync   string `mapstructure:"aws_psync" default:""` // 10.0.0.1:6379@nmfu2sl5osync,10.0.0.1:6379@xhma21xfkssync
}

func (opt *SyncReaderOptions) GetPSyncCommand(address string) string {
	items := strings.Split(opt.AwsPSync, ",")
	for _, item := range items {
		if strings.HasPrefix(item, address) {
			return strings.Split(item, "@")[1]
		}
	}
	log.Panicf("can not find aws psync command. address=[%s],aws_psync=[%s]", address, opt.AwsPSync)
	return ""
}

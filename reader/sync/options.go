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

// Tls with server tls.
func Tls(tls bool) SyncReaderOption {
	return func(s *SyncReaderOptions) {
		s.tls = tls
	}
}

// SyncRdb with server syncRdb.
func SyncRdb(syncRdb bool) SyncReaderOption {
	return func(s *SyncReaderOptions) {
		s.syncRdb = syncRdb
	}
}

// SyncAof with server syncAof.
func SyncAof(syncAof bool) SyncReaderOption {
	return func(s *SyncReaderOptions) {
		s.syncAof = syncAof
	}
}

// PreferReplica with server preferReplica.
func PreferReplica(preferReplica bool) SyncReaderOption {
	return func(s *SyncReaderOptions) {
		s.preferReplica = preferReplica
	}
}

// TryDiskless with server tryDiskless.
func TryDiskless(tryDiskless bool) SyncReaderOption {
	return func(s *SyncReaderOptions) {
		s.tryDiskless = tryDiskless
	}
}

// StatusPort with server statusPort.
func StatusPort(statusPort int) SyncReaderOption {
	return func(s *SyncReaderOptions) {
		s.statusPort = statusPort
	}
}

type SyncReaderOptions struct {
	cluster       bool   `mapstructure:"cluster" default:"false"`
	address       string `mapstructure:"address" default:""`
	username      string `mapstructure:"username" default:""`
	password      string `mapstructure:"password" default:""`
	tls           bool   `mapstructure:"tls" default:"false"`
	syncRdb       bool   `mapstructure:"sync_rdb" default:"true"`
	syncAof       bool   `mapstructure:"sync_aof" default:"true"`
	preferReplica bool   `mapstructure:"prefer_replica" default:"false"`
	tryDiskless   bool   `mapstructure:"try_diskless" default:"false"`

	// advanced
	statusPort int    `mapstructure:"status_port" default:"0"`
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

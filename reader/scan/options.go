package scan

type ScanReaderOptions struct {
	Cluster       bool   `mapstructure:"cluster" default:"false"`
	Address       string `mapstructure:"address" default:""`
	Username      string `mapstructure:"username" default:""`
	Password      string `mapstructure:"password" default:""`
	Tls           bool   `mapstructure:"tls" default:"false"`
	Scan          bool   `mapstructure:"scan" default:"true"`
	KSN           bool   `mapstructure:"ksn" default:"false"`
	DBS           []int  `mapstructure:"dbs"`
	PreferReplica bool   `mapstructure:"prefer_replica" default:"false"`
	Count         int    `mapstructure:"count" default:"1"`

	// advanced
	RDBRestoreCommandBehavior  string `mapstructure:"rdb_restore_command_behavior" default:"panic"`
	TargetRedisProtoMaxBulkLen uint64 `mapstructure:"target_redis_proto_max_bulk_len" default:"512000000"`
}

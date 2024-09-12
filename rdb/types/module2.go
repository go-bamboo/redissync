package types

import (
	"io"

	"github.com/go-bamboo/redissync/log"
	"github.com/go-bamboo/redissync/rdb/structure"
)

type ModuleObject interface {
	RedisObject
}

func PareseModuleType(rd io.Reader, key string, typeByte byte) ModuleObject {
	if typeByte == rdbTypeModule {
		log.Panicf("module type with version 1 is not supported, key=[%s]", key)
	}
	moduleId := structure.ReadLength(rd)
	moduleName := ModuleTypeNameByID(moduleId)
	switch moduleName {
	case "exstrtype":
		o := new(TairStringObject)
		o.LoadFromBuffer(rd, key, typeByte)
		return o
	case "tairhash-":
		o := new(TairHashObject)
		o.LoadFromBuffer(rd, key, typeByte)
		return o
	case "tairzset_":
		o := new(TairZsetObject)
		o.LoadFromBuffer(rd, key, typeByte)
		return o
	case "MBbloom--":
		// TODO: targetMBbloomVersion
		o := new(BloomObject)
		o.targetMBbloomVersion = 0
		o.encver = int(moduleId & 1023)
		o.LoadFromBuffer(rd, key, typeByte)
		return o
	default:
		log.Panicf("unsupported module type: %s", moduleName)
		return nil

	}
}

package filter

import (
	"github.com/go-bamboo/redissync/entry"
	"github.com/go-bamboo/redissync/log"
	"golang.org/x/exp/slices"
	"strings"
)

type FilterOptions struct {
	AllowKeyPrefix    []string `mapstructure:"allow_key_prefix" default:"[]"`
	AllowKeySuffix    []string `mapstructure:"allow_key_suffix" default:"[]"`
	BlockKeyPrefix    []string `mapstructure:"block_key_prefix" default:"[]"`
	BlockKeySuffix    []string `mapstructure:"block_key_suffix" default:"[]"`
	AllowDB           []int    `mapstructure:"allow_db" default:"[]"`
	BlockDB           []int    `mapstructure:"block_db" default:"[]"`
	AllowCommand      []string `mapstructure:"allow_command" default:"[]"`
	BlockCommand      []string `mapstructure:"block_command" default:"[]"`
	AllowCommandGroup []string `mapstructure:"allow_command_group" default:"[]"`
	BlockCommandGroup []string `mapstructure:"block_command_group" default:"[]"`
	Function          string   `mapstructure:"function" default:""`
}

// Filter returns:
// - true if the entry should be processed
// - false if it should be filtered out
func Filter(opt *FilterOptions, e *entry.Entry) bool {
	keyResults := make([]bool, len(e.Keys))
	for i := range keyResults {
		keyResults[i] = true
	}

	for inx, key := range e.Keys {
		// Check if the key matches any of the allowed patterns
		allow := false
		for _, prefix := range opt.AllowKeyPrefix {
			if strings.HasPrefix(key, prefix) {
				allow = true
			}
		}
		for _, suffix := range opt.AllowKeySuffix {
			if strings.HasSuffix(key, suffix) {
				allow = true
			}
		}
		if len(opt.AllowKeyPrefix) == 0 && len(opt.AllowKeySuffix) == 0 {
			allow = true
		}
		if !allow {
			keyResults[inx] = false
		}

		// Check if the key matches any of the blocked patterns
		block := false
		for _, prefix := range opt.BlockKeyPrefix {
			if strings.HasPrefix(key, prefix) {
				block = true
			}
		}
		for _, suffix := range opt.BlockKeySuffix {
			if strings.HasSuffix(key, suffix) {
				block = true
			}
		}
		if block {
			keyResults[inx] = false
		}
	}

	allTrue := true
	allFalse := true
	var passedKeys, filteredKeys []string
	for i, result := range keyResults {
		if result {
			allFalse = false
			passedKeys = append(passedKeys, e.Keys[i])
		} else {
			allTrue = false
			filteredKeys = append(filteredKeys, e.Keys[i])
		}
	}
	if allTrue {
		// All keys are allowed, continue checking
	} else if allFalse {
		return false
	} else {
		// If we reach here, it means some keys are true and some are false
		log.Debugf("Error: Inconsistent filter results for entry with %d keys: %v", len(e.Keys))
		log.Debugf("Passed keys: %v", passedKeys)
		log.Debugf("Filtered keys: %v", filteredKeys)
		return false
	}

	// Check if the database matches any of the allowed databases
	if len(opt.AllowDB) > 0 {
		if !slices.Contains(opt.AllowDB, e.DbId) {
			return false
		}
	}
	// Check if the database matches any of the blocked databases
	if len(opt.BlockDB) > 0 {
		if slices.Contains(opt.BlockDB, e.DbId) {
			return false
		}
	}

	// Check if the command matches any of the allowed commands
	if len(opt.AllowCommand) > 0 {
		if !slices.Contains(opt.AllowCommand, e.CmdName) {
			return false
		}
	}
	// Check if the command matches any of the blocked commands
	if len(opt.BlockCommand) > 0 {
		if slices.Contains(opt.BlockCommand, e.CmdName) {
			return false
		}
	}

	// Check if the command group matches any of the allowed command groups
	if len(opt.AllowCommandGroup) > 0 {
		if !slices.Contains(opt.AllowCommandGroup, e.Group) {
			return false
		}
	}
	// Check if the command group matches any of the blocked command groups
	if len(opt.BlockCommandGroup) > 0 {
		if slices.Contains(opt.BlockCommandGroup, e.Group) {
			return false
		}
	}

	return true
}

package terraform

import (
	"reflect"
)

type splitConfigs struct {
	Specific map[string]string
	Global   map[string]string
}

func splitConfig(nodeSpecificConfigKeys map[string]map[string]bool) func(m any, entity string) splitConfigs {
	return func(m any, entity string) splitConfigs {
		v := reflect.ValueOf(m)

		if v.Kind() != reflect.Map {
			panic("config is not a map")
		}

		if v.Type().Key().Kind() != reflect.String {
			panic("config key is not string")
		}

		lookup := nodeSpecificConfigKeys[entity]

		specific := map[string]string{}
		global := map[string]string{}

		iter := reflect.ValueOf(m).MapRange()
		for iter.Next() {
			k := iter.Key()
			v := iter.Value()

			if lookup[k.String()] {
				specific[k.String()] = v.String()
			} else {
				global[k.String()] = v.String()
			}
		}

		return splitConfigs{
			Specific: specific,
			Global:   global,
		}
	}
}

package session

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/lyraproj/dgo/dgo"
	json "github.com/lyraproj/dgo/streamer"
	"github.com/lyraproj/dgo/vf"
	"github.com/lyraproj/dgoyaml/yaml"
	"github.com/yanndegat/hiera/api"
)

var iplPattern = regexp.MustCompile(`%\{(?:[^{}]+|%\{(?:[^{}]+)*\})*\}`)
var emptyInterpolations = map[string]bool{
	``:     true,
	`::`:   true,
	`""`:   true,
	"''":   true,
	`"::"`: true,
	"'::'": true,
}

// Interpolate resolves interpolations in the given value and returns the result
func (ic *ivContext) Interpolate(value dgo.Value, allowMethods bool) dgo.Value {
	if result, changed := ic.doInterpolate(value, allowMethods); changed {
		return result
	}
	return value
}

func (ic *ivContext) doInterpolate(value dgo.Value, allowMethods bool) (dgo.Value, bool) {
	if s, ok := value.(dgo.String); ok {
		return ic.InterpolateString(s.String(), allowMethods)
	}
	if a, ok := value.(dgo.Array); ok {
		cp := a.AppendToSlice(make([]dgo.Value, 0, a.Len()))
		changed := false
		for i, e := range cp {
			v, c := ic.doInterpolate(e, allowMethods)
			if c {
				changed = true
				cp[i] = v
			}
		}
		if changed {
			a = vf.Array(cp)
		}
		return a, changed
	}
	if h, ok := value.(dgo.Map); ok {
		cp := vf.MapWithCapacity(h.Len())
		changed := false
		h.EachEntry(func(e dgo.MapEntry) {
			k, kc := ic.doInterpolate(e.Key(), allowMethods)
			v, vc := ic.doInterpolate(e.Value(), allowMethods)
			cp.Put(k, v)
			if kc || vc {
				changed = true
			}
		})
		if changed {
			cp.Freeze()
			h = cp
		}
		return h, changed
	}
	return value, false
}

type iplMethod int
type iplEncodeMethod int

const (
	scopeMethod = iplMethod(iota)
	aliasMethod
	strictAliasMethod
	lookupMethod
	literalMethod
	yamlEncode = iplEncodeMethod(iota)
	jsonEncode
)

func (m iplMethod) isAlias() bool {
	return m == aliasMethod || m == strictAliasMethod
}

var methodMatch = regexp.MustCompile(`^(\w+)\((?:["]([^"]+)["]|[']([^']+)['])\)(\|(yaml|json))?$`)

func getMethodAndData(expr string, allowMethods bool) (iplMethod, string, iplEncodeMethod) {
	var encodeMethod iplEncodeMethod
	if groups := methodMatch.FindStringSubmatch(expr); groups != nil {
		if !allowMethods {
			panic(errors.New(`interpolation using method syntax is not allowed in this context`))
		}
		data := groups[2]
		if data == `` {
			data = groups[3]
		}

		switch groups[5] {
		case `yaml`:
			encodeMethod = yamlEncode
		case `json`:
			encodeMethod = jsonEncode
		default:
		}

		switch groups[1] {
		case `alias`:
			return aliasMethod, data, encodeMethod
		case `strict_alias`:
			return strictAliasMethod, data, encodeMethod
		case `hiera`, `lookup`:
			return lookupMethod, data, encodeMethod
		case `literal`:
			return literalMethod, data, encodeMethod
		case `scope`:
			return scopeMethod, data, encodeMethod
		default:
			panic(fmt.Errorf(`unknown interpolation method '%s'`, groups[1]))
		}
	}
	return scopeMethod, expr, encodeMethod
}

// InterpolateString resolves a string containing interpolation expressions
func (ic *ivContext) InterpolateString(str string, allowMethods bool) (dgo.Value, bool) {
	if !strings.Contains(str, `%{`) {
		return vf.String(str), false
	}

	return ic.WithInterpolation(str, func() dgo.Value {
		var result dgo.Value
		var methodKey iplMethod
		var encodeMethod iplEncodeMethod

		str = iplPattern.ReplaceAllStringFunc(str, func(match string) string {
			expr := strings.TrimSpace(match[2 : len(match)-1])
			if emptyInterpolations[expr] {
				return ``
			}

			methodKey, expr, encodeMethod = getMethodAndData(expr, allowMethods)
			if methodKey.isAlias() && match != str {
				panic(errors.New(`'alias'/'strict_alias' interpolation is only permitted if the expression is equal to the entire string`))
			}

			switch methodKey {
			case literalMethod:
				return expr
			case scopeMethod:
				if val := ic.InterpolateInScope(expr, allowMethods); val != nil {
					return val.String()
				}
				return ``
			default:
				val := ic.Lookup(api.NewKey(expr), nil)
				switch encodeMethod {
				case jsonEncode:
					if val.Equals(vf.Nil) {
						return `null`
					}
					return string(json.MarshalJSON(val, nil))
				case yamlEncode:
					if val.Equals(vf.Nil) {
						return ``
					}
					bs, err := yaml.Marshal(val)
					if err != nil {
						panic(err)
					}
					return string(bs)
				default:
					if methodKey.isAlias() {
						result = val
						return ``
					}
					if val == nil {
						return ``
					}
					return val.String()
				}
			}
		})
		if result == nil && methodKey != strictAliasMethod {
			result = vf.String(str)
		}

		return result
	}), true
}

// InterpolateInScope resolves a key expression in the invocation scope
func (ic *ivContext) InterpolateInScope(expr string, allowMethods bool) dgo.Value {
	key := api.NewKey(expr)
	if val := ic.Scope().Get(key.Root()); val != nil {
		val, _ = ic.doInterpolate(val, allowMethods)
		return key.Dig(ic, val)
	}
	return nil
}

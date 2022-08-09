package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const SignKey = "sign"

func Verify(salt string, arguments map[string]string) bool {
	sign := arguments[SignKey]
	data := joinArguments(arguments) + salt
	h := md5.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil)) == sign
}

func Create(salt string, arguments interface{}) string {
	data := joinArguments(arguments) + salt
	h := md5.New()
	h.Write([]byte(data))

	return hex.EncodeToString(h.Sum(nil))
}

// 排序并合并参数拼成字符串
func joinArguments(data interface{}) string {
	if val, ok := data.(url.Values); ok {
		if val == nil {
			return ""
		}
		var buf strings.Builder
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == SignKey {
				continue
			}
			vs := val[k]
			for _, v := range vs {
				if buf.Len() > 0 {
					buf.WriteByte('&')
				}
				buf.WriteString(k)
				buf.WriteByte('=')
				buf.WriteString(v)
			}
		}
		return buf.String()
	}

	rType := reflect.TypeOf(data)
	rValue := reflect.ValueOf(data)

	var keys []string
	var keyMapper = make(map[string]int)
	var keyMapper2 = make(map[string]reflect.Value)
	var r func(reflect.Value, string)
	var result []string

	switch rType.Kind() {
	case reflect.Map:
		for _, k := range rValue.MapKeys() {
			if String(k) == SignKey {
				continue
			}
			keyMapper2[String(k)] = k
			keys = append(keys, String(k))
		}
	case reflect.Struct:
		for i := 0; i < rType.NumField(); i++ {
			fType := rType.Field(i)
			tags := strings.TrimSpace(fType.Tag.Get("json"))
			if tags == SignKey || tags == "-" {
				continue
			}
			keys = append(keys, fType.Tag.Get("json"))
			keyMapper[tags] = i
		}
	}

	sort.Strings(keys)
	r = func(vv reflect.Value, father string) {
		dat := vv.Interface()
		v := reflect.ValueOf(dat)
		t := v.Type()
		switch t.Kind() {
		case reflect.Array, reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				r(v.Index(i), father+"["+String(i)+"]")
			}
		case reflect.Chan, reflect.Func, reflect.Struct, reflect.Map: // map, struct 不保序，上下游不可使用
			return
		default:
			result = append(result, fmt.Sprintf("%s=%s", father, String(v.Interface())))
		}
	}

	for _, key := range keys {
		switch rType.Kind() {
		case reflect.Map:
			r(rValue.MapIndex(keyMapper2[key]), key)
		case reflect.Struct:
			r(rValue.Field(keyMapper[key]), key)
		}
	}

	return strings.Join(result, "&")
}

func String(src interface{}) string {
	switch val := src.(type) {
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case string:
		return val
	case []byte:
		return string(val)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", src)
	}
}

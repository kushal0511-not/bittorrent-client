package util

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
)

func EncodeBencode(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return EncodeString(v)
	case int:
		return EncodeNumber(v)
	case []interface{}:
		return EncodeList(v)
	case map[string]interface{}:
		return EncodeDictionary(v)
	default:
		return "", fmt.Errorf("unsupported type: %T", v)
	}
}
func EncodeString(arg string) (string, error) {
	return strconv.Itoa(len(arg)) + ":" + arg, nil
}
func EncodeNumber(arg int) (string, error) {
	return "i" + strconv.Itoa(arg) + "e", nil
}
func EncodeList(arg []interface{}) (string, error) {
	var buffer bytes.Buffer
	buffer.WriteString("l")
	for _, item := range arg {
		encodedItem, err := EncodeBencode(item)
		if err != nil {
			return "", err
		}
		buffer.WriteString(encodedItem)
	}
	buffer.WriteString("e")
	return buffer.String(), nil
}
func EncodeDictionary(arg map[string]interface{}) (string, error) {
	var buffer bytes.Buffer
	buffer.WriteString("d")
	// Keys must be sorted
	keys := make([]string, 0, len(arg))
	for key := range arg {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		encodedKey, _ := EncodeBencode(key) // Since key is always a string, no error handling needed here
		encodedValue, err := EncodeBencode(arg[key])
		if err != nil {
			return "", err
		}
		buffer.WriteString(encodedKey)
		buffer.WriteString(encodedValue)
	}
	buffer.WriteString("e")
	return buffer.String(), nil
}

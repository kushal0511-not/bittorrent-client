package util

import (
	"fmt"
	"strconv"
	"unicode"
)

func DecodeBencode(bencodedString string, partition int) (interface{}, int, error) {
	switch {
	case bencodedString[partition] == 'i':
		return DecodeInt(bencodedString, partition)
	case bencodedString[partition] == 'l':
		return DecodeList(bencodedString, partition)
	case bencodedString[partition] >= '0' && bencodedString[partition] <= '9':
		return DecodeString(bencodedString, partition)
	case bencodedString[partition] == 'd':
		return DecodeDict(bencodedString, partition)
	}
	return nil, 0, nil
}
func DecodeString(bencodedString string, partition int) (interface{}, int, error) {
	if unicode.IsDigit(rune(bencodedString[partition])) {
		var firstColonIndex int
		i := partition
		// fmt.Println(i)
		for {
			if i >= len(bencodedString) {
				return "", partition, fmt.Errorf("invalid index")
			}
			if bencodedString[i] == ':' {
				firstColonIndex = i
				break
			}
			i = i + 1
			// fmt.Println(i)
		}
		lengthStr := bencodedString[partition:firstColonIndex]

		length, err := strconv.Atoi(lengthStr)

		if err != nil {
			return "", partition, err
		}
		return bencodedString[firstColonIndex+1 : firstColonIndex+1+length], firstColonIndex + length, nil
	} else {
		return "", partition, fmt.Errorf("only strings are supported at the moment")
	}
}
func DecodeList(bencodedString string, partition int) (interface{}, int, error) {
	var list = make([]interface{}, 0, len(bencodedString))
	for partition+1 < len(bencodedString) && bencodedString[partition+1] != 'e' {
		// fmt.Println("partition", partition+1)
		item, newPartition, err := DecodeBencode(bencodedString, partition+1)
		partition = newPartition
		// fmt.Println("partition", item)
		if err != nil {
			return nil, partition, err
		}
		list = append(list, item)
	}
	// fmt.Println("hello")
	return list, partition + 1, nil
}
func DecodeInt(bencodedString string, partition int) (interface{}, int, error) {
	i := partition
	for {
		if bencodedString[i] == 'e' {
			break
		}
		i = i + 1
	}
	numberStr := bencodedString[partition+1 : i]
	number, err := strconv.Atoi(numberStr)
	return number, i, err
}
func DecodeDict(bencodedString string, partition int) (interface{}, int, error) {
	var dict = make(map[string]interface{})
	for partition+1 < len(bencodedString) && bencodedString[partition+1] != 'e' {
		key, newPartition, err := DecodeBencode(bencodedString, partition+1)
		if err != nil {
			return nil, partition, err
		}
		value, newestPartition, err := DecodeBencode(bencodedString, newPartition+1)
		dict[key.(string)] = value
		partition = newestPartition
		// fmt.Println("partition", item)
		if err != nil {
			return nil, partition, err
		}
	}
	// fmt.Println("hello")
	return dict, partition + 1, nil
}

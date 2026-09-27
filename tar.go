// Package tarreader 解析 ustar 头。
package tarreader

import (
	"fmt"
	"strconv"
	"strings"
)

// Header 是一个 ustar 头。
type Header struct {
	Name     string
	Size     int64
	Mode     int64
	Typeflag byte
	Checksum int64
}

// Parse 解析 512 字节头。缺陷：校验和把该字段当数字算（应为 8 个空格），长名也不拼 prefix。
func Parse(block []byte) (*Header, error) {
	if len(block) < 512 {
		return nil, fmt.Errorf("头必须是 512 字节，实际 %d", len(block))
	}
	header := &Header{
		Name:     cString(block[0:100]),
		Mode:     octalOrZero(block[100:108]),
		Size:     octalOrZero(block[124:136]),
		Typeflag: block[156],
		Checksum: octalOrZero(block[148:156]),
	}
	// 缺陷：没有把 chksum 字段替换成 8 个空格再求和
	sum := int64(0)
	for _, byte := range block {
		sum += int64(byte)
	}
	if sum != header.Checksum {
		return nil, fmt.Errorf("校验和错误：算出 %d，头部写着 %d", sum, header.Checksum)
	}
	return header, nil
}

func cString(field []byte) string {
	return strings.TrimRight(string(field), "\x00")
}

func octalOrZero(field []byte) int64 {
	text := strings.Trim(string(field), "\x00 ")
	if text == "" {
		return 0
	}
	value, err := strconv.ParseInt(text, 8, 64)
	if err != nil {
		return 0
	}
	return value
}

// BuildHeader 按 ustar 规范造一个头（校验和字段按 8 个空格参与计算）。
func BuildHeader(name string, size int64, prefix string) []byte {
	block := make([]byte, 512)
	copy(block[0:100], name)
	copy(block[100:108], "0000644\x00")
	copy(block[108:116], "0000000\x00")
	copy(block[116:124], "0000000\x00")
	copy(block[124:136], fmt.Sprintf("%011o\x00", size))
	copy(block[136:148], "00000000000\x00")
	for index := 148; index < 156; index++ {
		block[index] = ' '
	}
	block[156] = '0'
	copy(block[257:263], "ustar\x00")
	copy(block[263:265], "00")
	copy(block[265:297], "root")
	copy(block[297:329], "root")
	copy(block[345:500], prefix)
	sum := int64(0)
	for _, byte := range block {
		sum += int64(byte)
	}
	copy(block[148:156], fmt.Sprintf("%06o\x00 ", sum))
	return block
}

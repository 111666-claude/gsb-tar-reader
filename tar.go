// Package tarreader 解析 ustar 头。
package tarreader

import (
	"bytes"
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
	Linkname string
	Checksum int64
}

const blockSize = 512

var zeroBlock [blockSize]byte

// Parse 解析 512 字节 ustar 头。
func Parse(block []byte) (*Header, error) {
	if len(block) < blockSize {
		return nil, fmt.Errorf("头必须是 512 字节，实际 %d", len(block))
	}
	block = block[:blockSize]
	if bytes.Equal(block, zeroBlock[:]) {
		return &Header{}, nil
	}
	checksum, err := octalField("chksum", block[148:156])
	if err != nil {
		return nil, err
	}
	// ustar 规定：求和时 chksum 字段按 8 个空格计算。
	sum := int64(0)
	for index, value := range block {
		if index >= 148 && index < 156 {
			sum += int64(' ')
		} else {
			sum += int64(value)
		}
	}
	if sum != checksum {
		return nil, fmt.Errorf("校验和错误：算出 %d，头部写着 %d", sum, checksum)
	}
	mode, err := octalField("mode", block[100:108])
	if err != nil {
		return nil, err
	}
	if _, err := octalField("uid", block[108:116]); err != nil {
		return nil, err
	}
	if _, err := octalField("gid", block[116:124]); err != nil {
		return nil, err
	}
	size, err := octalField("size", block[124:136])
	if err != nil {
		return nil, err
	}
	if size < 0 {
		return nil, fmt.Errorf("文件大小为负：%d", size)
	}
	if _, err := octalField("mtime", block[136:148]); err != nil {
		return nil, err
	}
	typeflag := block[156]
	switch typeflag {
	case '0', '\x00', '5', '2':
	default:
		return nil, fmt.Errorf("未知类型标志：%q", typeflag)
	}
	name := cString(block[0:100])
	if prefix := cString(block[345:500]); prefix != "" {
		name = prefix + "/" + name
	}
	header := &Header{
		Name:     name,
		Size:     size,
		Mode:     mode,
		Typeflag: typeflag,
		Checksum: checksum,
	}
	if typeflag == '2' {
		header.Linkname = cString(block[157:257])
	}
	return header, nil
}

func cString(field []byte) string {
	return strings.TrimRight(string(field), "\x00")
}

// octalField 把八进制 ASCII 字段（NUL 或空格结尾）解析成整数。
func octalField(fieldName string, field []byte) (int64, error) {
	text := strings.Trim(string(field), "\x00 ")
	if text == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(text, 8, 64)
	if err != nil {
		return 0, fmt.Errorf("字段 %s 不是合法八进制：%q", fieldName, text)
	}
	return value, nil
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

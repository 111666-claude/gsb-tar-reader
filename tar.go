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
	Uid      int64
	Gid      int64
	Mtime    int64
	Typeflag byte
	Linkname string
	Checksum int64
}

// Parse 解析 512 字节 ustar 头。
func Parse(block []byte) (*Header, error) {
	if len(block) < 512 {
		return nil, fmt.Errorf("头必须是 512 字节，实际 %d", len(block))
	}
	if isZeroBlock(block[:512]) {
		return &Header{}, nil
	}
	header := &Header{}
	var err error
	if header.Mode, err = octalField(block[100:108], "mode"); err != nil {
		return nil, err
	}
	if header.Uid, err = octalField(block[108:116], "uid"); err != nil {
		return nil, err
	}
	if header.Gid, err = octalField(block[116:124], "gid"); err != nil {
		return nil, err
	}
	if header.Size, err = octalField(block[124:136], "size"); err != nil {
		return nil, err
	}
	if header.Size < 0 {
		return nil, fmt.Errorf("字段 size 不能为负：%d", header.Size)
	}
	if header.Mtime, err = octalField(block[136:148], "mtime"); err != nil {
		return nil, err
	}
	if header.Checksum, err = octalField(block[148:156], "chksum"); err != nil {
		return nil, err
	}
	// ustar 规定：求和时 chksum 字段要当成 8 个空格。
	sum := int64(0)
	for index, value := range block[:512] {
		if index >= 148 && index < 156 {
			sum += int64(' ')
		} else {
			sum += int64(value)
		}
	}
	if sum != header.Checksum {
		return nil, fmt.Errorf("校验和错误：算出 %d，头部写着 %d", sum, header.Checksum)
	}
	name := cString(block[0:100])
	if prefix := cString(block[345:500]); prefix != "" {
		name = prefix + "/" + name
	}
	header.Name = name
	switch header.Typeflag = block[156]; header.Typeflag {
	case 0, '0': // 普通文件
	case '5': // 目录
	case '2': // 符号链接，linkname 是目标
		header.Linkname = cString(block[157:257])
	default:
		return nil, fmt.Errorf("未知类型标志：%q", header.Typeflag)
	}
	return header, nil
}

func isZeroBlock(block []byte) bool {
	for _, value := range block {
		if value != 0 {
			return false
		}
	}
	return true
}

func cString(field []byte) string {
	return strings.TrimRight(string(field), "\x00")
}

func octalField(field []byte, name string) (int64, error) {
	text := strings.Trim(string(field), "\x00 ")
	if text == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(text, 8, 64)
	if err != nil {
		return 0, fmt.Errorf("字段 %s 不是合法八进制：%q", name, text)
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
	for _, value := range block {
		sum += int64(value)
	}
	copy(block[148:156], fmt.Sprintf("%06o\x00 ", sum))
	return block
}

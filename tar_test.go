package tarreader

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestParseRejectsShortBlock(t *testing.T) {
	if _, err := Parse(make([]byte, 100)); err == nil {
		t.Fatal("不足 512 字节应该报错")
	}
}

func TestBuildHeaderIs512Bytes(t *testing.T) {
	if got := len(BuildHeader("a.txt", 10, "")); got != 512 {
		t.Fatalf("头应该是 512 字节：%d", got)
	}
}

func TestParseZeroBlock(t *testing.T) {
	header, err := Parse(make([]byte, 512))
	if err != nil {
		t.Fatalf("全零头应该能解析：%v", err)
	}
	if header.Name != "" || header.Size != 0 {
		t.Fatalf("全零头不该有名字与大小：%+v", header)
	}
}

func TestBuildHeaderKeepsName(t *testing.T) {
	block := BuildHeader("hello.txt", 3, "")
	if string(block[:9]) != "hello.txt" {
		t.Fatalf("name 字段不对：%q", string(block[:9]))
	}
}

func TestBuildHeaderKeepsPrefix(t *testing.T) {
	block := BuildHeader("f.txt", 1, "deep/dir")
	if string(block[345:353]) != "deep/dir" {
		t.Fatalf("prefix 字段不对：%q", string(block[345:353]))
	}
}

// fixChecksum 按规范算法（chksum 字段当 8 个空格）重写校验和。
func fixChecksum(block []byte) {
	for index := 148; index < 156; index++ {
		block[index] = ' '
	}
	sum := int64(0)
	for _, value := range block {
		sum += int64(value)
	}
	copy(block[148:156], fmt.Sprintf("%06o\x00 ", sum))
}

func TestParseBuiltHeaderChecksum(t *testing.T) {
	header, err := Parse(BuildHeader("file.txt", 42, ""))
	if err != nil {
		t.Fatalf("合法头不该报校验和错误：%v", err)
	}
	if header.Name != "file.txt" || header.Size != 42 {
		t.Fatalf("name/size 不对：%+v", header)
	}
}

func TestChecksumErrorReportsBothNumbers(t *testing.T) {
	block := BuildHeader("file.txt", 42, "")
	block[0] = 'X' // 破坏内容但不重算校验和
	_, err := Parse(block)
	if err == nil {
		t.Fatal("校验和不对应该报错")
	}
	message := err.Error()
	if !strings.Contains(message, "算出") || !strings.Contains(message, "头部写着") {
		t.Fatalf("错误信息要带上两个数字：%q", message)
	}
}

func TestLongNameJoinsPrefix(t *testing.T) {
	header, err := Parse(BuildHeader("long.txt", 1, "deep/nested/path"))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if header.Name != "deep/nested/path/long.txt" {
		t.Fatalf("长名应该拼 prefix：%q", header.Name)
	}
}

func TestEmptyPrefixUsesNameOnly(t *testing.T) {
	header, err := Parse(BuildHeader("plain.txt", 1, ""))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if header.Name != "plain.txt" {
		t.Fatalf("prefix 为空时只用 name：%q", header.Name)
	}
}

func TestOctalBoundaries(t *testing.T) {
	cases := []struct {
		name string
		size int64
	}{
		{"zero.bin", 0},
		{"one.bin", 1},
		{"max.bin", 0o77777777777}, // 11 位八进制上限
	}
	for _, testCase := range cases {
		header, err := Parse(BuildHeader(testCase.name, testCase.size, ""))
		if err != nil {
			t.Fatalf("%s：解析失败：%v", testCase.name, err)
		}
		if header.Size != testCase.size {
			t.Fatalf("%s：size 应为 %d，实际 %d", testCase.name, testCase.size, header.Size)
		}
		if header.Mode != 0o644 {
			t.Fatalf("%s：mode 应为 0644，实际 %o", testCase.name, header.Mode)
		}
	}
}

func TestInvalidOctalFieldNamesField(t *testing.T) {
	fields := []struct {
		name  string
		start int
	}{
		{"mode", 100},
		{"uid", 108},
		{"gid", 116},
		{"size", 124},
		{"mtime", 136},
	}
	for _, field := range fields {
		block := BuildHeader("f.txt", 1, "")
		copy(block[field.start:field.start+8], "abcdefg\x00")
		fixChecksum(block)
		_, err := Parse(block)
		if err == nil {
			t.Fatalf("字段 %s 非法应该报错", field.name)
		}
		if !strings.Contains(err.Error(), field.name) {
			t.Fatalf("错误要指出字段名 %s：%q", field.name, err.Error())
		}
	}
}

func TestNegativeSizeRejected(t *testing.T) {
	block := BuildHeader("f.txt", 1, "")
	copy(block[124:136], "-0000000001\x00")
	fixChecksum(block)
	if _, err := Parse(block); err == nil {
		t.Fatal("文件大小为负应该报错")
	}
}

func TestTypeflags(t *testing.T) {
	cases := []struct {
		typeflag byte
		linkname string
	}{
		{'0', ""},
		{'\x00', ""},
		{'5', ""},
		{'2', "target/file"},
	}
	for _, testCase := range cases {
		block := BuildHeader("f.txt", 0, "")
		block[156] = testCase.typeflag
		if testCase.linkname != "" {
			copy(block[157:257], testCase.linkname)
		}
		fixChecksum(block)
		header, err := Parse(block)
		if err != nil {
			t.Fatalf("类型标志 %q 应该能解析：%v", testCase.typeflag, err)
		}
		if header.Typeflag != testCase.typeflag {
			t.Fatalf("类型标志应为 %q，实际 %q", testCase.typeflag, header.Typeflag)
		}
		if header.Linkname != testCase.linkname {
			t.Fatalf("linkname 应为 %q，实际 %q", testCase.linkname, header.Linkname)
		}
	}
}

func TestUnknownTypeflagRejected(t *testing.T) {
	block := BuildHeader("f.txt", 1, "")
	block[156] = '9'
	fixChecksum(block)
	if _, err := Parse(block); err == nil {
		t.Fatal("未知类型标志应该报错")
	}
}

func TestRoundTripKeepsNameAndSize(t *testing.T) {
	cases := []struct {
		name   string
		size   int64
		prefix string
	}{
		{"a.txt", 0, ""},
		{"hello.txt", 42, ""},
		{"long.txt", 1234567, "deep/nested/path"},
	}
	for _, testCase := range cases {
		header, err := Parse(BuildHeader(testCase.name, testCase.size, testCase.prefix))
		if err != nil {
			t.Fatalf("解析失败：%v", err)
		}
		wantName := testCase.name
		if testCase.prefix != "" {
			wantName = testCase.prefix + "/" + testCase.name
		}
		if header.Name != wantName || header.Size != testCase.size {
			t.Fatalf("往返不一致：got name=%q size=%d，want name=%q size=%d",
				header.Name, header.Size, wantName, testCase.size)
		}
	}
}

func TestParseIsDeterministic(t *testing.T) {
	block := BuildHeader("f.txt", 9, "p")
	first, err := Parse(block)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	second, err := Parse(block)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if *first != *second {
		t.Fatalf("同一块重复解析结果不同：%+v vs %+v", first, second)
	}
}

// repeatReader 无限重复同一个 512 字节块。
type repeatReader struct {
	block []byte
}

func (reader *repeatReader) Read(buffer []byte) (int, error) {
	copied := 0
	for copied < len(buffer) {
		copied += copy(buffer[copied:], reader.block)
	}
	return copied, nil
}

func TestParseMillionHeadersSinglePass(t *testing.T) {
	reader := &repeatReader{block: BuildHeader("file.txt", 42, "deep")}
	buffer := make([]byte, 512)
	for index := 0; index < 1_000_000; index++ {
		if _, err := io.ReadFull(reader, buffer); err != nil {
			t.Fatalf("第 %d 个头读取失败：%v", index, err)
		}
		header, err := Parse(buffer)
		if err != nil {
			t.Fatalf("第 %d 个头解析失败：%v", index, err)
		}
		if header.Name != "deep/file.txt" || header.Size != 42 {
			t.Fatalf("第 %d 个头不对：%+v", index, header)
		}
	}
}

func TestCompareWithArchiveTar(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	randomString := func(length int) string {
		var builder strings.Builder
		for builder.Len() < length {
			builder.WriteByte(byte('a' + rng.Intn(26)))
		}
		return builder.String()
	}
	for iteration := 0; iteration < 1000; iteration++ {
		want := tar.Header{
			Format:  tar.FormatUSTAR,
			Mode:    rng.Int63n(0o7777 + 1),
			Uid:     rng.Intn(65536),
			Gid:     rng.Intn(65536),
			ModTime: time.Unix(rng.Int63n(1<<31), 0),
		}
		name := randomString(1 + rng.Intn(100))
		if prefixLength := rng.Intn(156); prefixLength > 0 {
			name = randomString(prefixLength) + "/" + name
		}
		want.Name = name
		switch rng.Intn(3) {
		case 0:
			want.Typeflag = tar.TypeReg
			want.Size = rng.Int63n(1 << 33) // 含大文件
		case 1:
			want.Typeflag = tar.TypeDir
		case 2:
			want.Typeflag = tar.TypeSymlink
			want.Linkname = randomString(1 + rng.Intn(100))
		}
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		if err := writer.WriteHeader(&want); err != nil {
			t.Fatalf("第 %d 组：archive/tar 造头失败：%v", iteration, err)
		}
		block := buffer.Bytes()
		if len(block) < 512 {
			t.Fatalf("第 %d 组：头不足 512 字节", iteration)
		}
		got, err := Parse(block[:512])
		if err != nil {
			t.Fatalf("第 %d 组（%q）：解析失败：%v", iteration, want.Name, err)
		}
		reader := tar.NewReader(bytes.NewReader(block))
		reference, err := reader.Next()
		if err != nil {
			t.Fatalf("第 %d 组：archive/tar 解析失败：%v", iteration, err)
		}
		if got.Name != reference.Name {
			t.Fatalf("第 %d 组：name 不一致：%q vs %q", iteration, got.Name, reference.Name)
		}
		if got.Size != reference.Size {
			t.Fatalf("第 %d 组：size 不一致：%d vs %d", iteration, got.Size, reference.Size)
		}
		if got.Mode != reference.Mode {
			t.Fatalf("第 %d 组：mode 不一致：%o vs %o", iteration, got.Mode, reference.Mode)
		}
		if got.Typeflag != reference.Typeflag {
			t.Fatalf("第 %d 组：typeflag 不一致：%q vs %q", iteration, got.Typeflag, reference.Typeflag)
		}
		if got.Linkname != reference.Linkname {
			t.Fatalf("第 %d 组：linkname 不一致：%q vs %q", iteration, got.Linkname, reference.Linkname)
		}
	}
}

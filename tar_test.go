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

func TestParseChecksumValid(t *testing.T) {
	header, err := Parse(BuildHeader("file.txt", 42, ""))
	if err != nil {
		t.Fatalf("合法头不该报校验和错误：%v", err)
	}
	if header.Checksum != 4762 {
		t.Fatalf("校验和应该是 4762：%d", header.Checksum)
	}
}

func TestParseChecksumMismatch(t *testing.T) {
	block := BuildHeader("file.txt", 42, "")
	block[0] = 'X'
	_, err := Parse(block)
	if err == nil {
		t.Fatal("改过的头应该报校验和错误")
	}
	message := err.Error()
	if !strings.Contains(message, "算出") || !strings.Contains(message, "头部写着") {
		t.Fatalf("错误信息要带上「算出」与「头部写着」：%q", message)
	}
}

func TestParseLongName(t *testing.T) {
	header, err := Parse(BuildHeader("long.txt", 1, "deep/nested/path"))
	if err != nil {
		t.Fatal(err)
	}
	if header.Name != "deep/nested/path/long.txt" {
		t.Fatalf("长名要拼 prefix：%q", header.Name)
	}
}

func TestParseOctalBoundaries(t *testing.T) {
	block := BuildHeader("big.bin", 0, "")
	copy(block[124:136], "77777777777\x00") // ustar size 上限 8G-1
	fixChecksum(block)
	header, err := Parse(block)
	if err != nil {
		t.Fatal(err)
	}
	if header.Size != 0o77777777777 {
		t.Fatalf("size 应该是 8G-1：%d", header.Size)
	}

	block = BuildHeader("space.bin", 1, "")
	copy(block[124:136], "       101 \x00") // 空格填充也合法
	fixChecksum(block)
	header, err = Parse(block)
	if err != nil {
		t.Fatal(err)
	}
	if header.Size != 0o101 {
		t.Fatalf("size 应该是 65：%d", header.Size)
	}
}

func TestParseInvalidNumericField(t *testing.T) {
	for _, field := range []struct {
		name  string
		start int
	}{
		{"mode", 100},
		{"uid", 108},
		{"gid", 116},
		{"size", 124},
		{"mtime", 136},
	} {
		block := BuildHeader("f.txt", 1, "")
		copy(block[field.start:], "zzzz")
		_, err := Parse(block)
		if err == nil {
			t.Fatalf("字段 %s 非法应该报错", field.name)
		}
		if !strings.Contains(err.Error(), field.name) {
			t.Fatalf("错误要指出字段名 %s：%q", field.name, err)
		}
	}
}

func TestParseNegativeSize(t *testing.T) {
	block := BuildHeader("f.txt", 1, "")
	copy(block[124:136], "-0000000001\x00")
	fixChecksum(block)
	if _, err := Parse(block); err == nil {
		t.Fatal("size 为负应该报错")
	}
}

func fixChecksum(block []byte) {
	for index := 148; index < 156; index++ {
		block[index] = ' '
	}
	sum := int64(0)
	for index, value := range block {
		if index >= 148 && index < 156 {
			sum += int64(' ')
		} else {
			sum += int64(value)
		}
	}
	copy(block[148:156], fmt.Sprintf("%06o\x00 ", sum))
}

func TestParseTypeflags(t *testing.T) {
	newBlock := func(typeflag byte) []byte {
		block := BuildHeader("f.txt", 1, "")
		block[156] = typeflag
		fixChecksum(block)
		return block
	}
	for _, typeflag := range []byte{'0', 0, '5'} {
		if _, err := Parse(newBlock(typeflag)); err != nil {
			t.Fatalf("类型标志 %q 应该合法：%v", typeflag, err)
		}
	}
	block := newBlock('2')
	copy(block[157:257], "target/of/link\x00")
	fixChecksum(block)
	header, err := Parse(block)
	if err != nil {
		t.Fatal(err)
	}
	if header.Linkname != "target/of/link" {
		t.Fatalf("符号链接要给出目标：%q", header.Linkname)
	}
	if _, err := Parse(newBlock('x')); err == nil {
		t.Fatal("未知类型标志应该报错")
	}
}

func TestRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 1000; i++ {
		name := randomString(rng, 1+rng.Intn(100))
		prefix := randomString(rng, rng.Intn(156))
		size := rng.Int63n(0o77777777777 + 1)
		header, err := Parse(BuildHeader(name, size, prefix))
		if err != nil {
			t.Fatalf("第 %d 组：%v", i, err)
		}
		want := name
		if prefix != "" {
			want = prefix + "/" + name
		}
		if header.Name != want || header.Size != size {
			t.Fatalf("第 %d 组不一致：got (%q, %d)，want (%q, %d)", i, header.Name, header.Size, want, size)
		}
	}
}

func TestParseDeterministic(t *testing.T) {
	block := BuildHeader("a/b.txt", 99, "p")
	first, err := Parse(block)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Parse(block)
	if err != nil {
		t.Fatal(err)
	}
	if *first != *second {
		t.Fatalf("同一块重复解析结果不同：%+v vs %+v", first, second)
	}
}

// repeatReader 无限重复同一个块，用来模拟超大归档。
type repeatReader struct{ block []byte }

func (r repeatReader) Read(p []byte) (int, error) {
	for n := 0; n < len(p); n += copy(p[n:], r.block) {
	}
	return len(p), nil
}

func TestParseMillionHeadersStreaming(t *testing.T) {
	reader := repeatReader{BuildHeader("stream.bin", 7, "p")}
	buffer := make([]byte, 512)
	for i := 0; i < 1_000_000; i++ {
		if _, err := io.ReadFull(reader, buffer); err != nil {
			t.Fatal(err)
		}
		header, err := Parse(buffer)
		if err != nil {
			t.Fatalf("第 %d 个头：%v", i, err)
		}
		if header.Name != "p/stream.bin" || header.Size != 7 {
			t.Fatalf("第 %d 个头不对：%+v", i, header)
		}
	}
}

func TestCompareWithArchiveTar(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 1000; i++ {
		want := randomTarHeader(rng)
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		if err := writer.WriteHeader(want); err != nil {
			t.Fatalf("第 %d 组写不进去：%v", i, err)
		}
		got, err := Parse(buffer.Bytes()[:512])
		if err != nil {
			t.Fatalf("第 %d 组（%+v）：%v", i, want, err)
		}
		readBack, err := tar.NewReader(bytes.NewReader(buffer.Bytes())).Next()
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != readBack.Name ||
			got.Size != readBack.Size ||
			got.Mode != readBack.Mode ||
			got.Uid != int64(readBack.Uid) ||
			got.Gid != int64(readBack.Gid) ||
			got.Mtime != readBack.ModTime.Unix() ||
			got.Typeflag != readBack.Typeflag ||
			got.Linkname != readBack.Linkname {
			t.Fatalf("第 %d 组不一致：\n got %+v\nwant %+v", i, got, readBack)
		}
	}
}

func randomTarHeader(rng *rand.Rand) *tar.Header {
	name := randomString(rng, 1+rng.Intn(100))
	if prefix := randomString(rng, rng.Intn(156)); prefix != "" {
		name = prefix + "/" + name
	}
	header := &tar.Header{
		Name:    name,
		Mode:    rng.Int63n(0o1000),
		Uid:     rng.Intn(65536),
		Gid:     rng.Intn(65536),
		ModTime: time.Unix(rng.Int63n(0o77777777777), 0),
		Format:  tar.FormatUSTAR,
	}
	switch rng.Intn(3) {
	case 0:
		header.Typeflag = tar.TypeReg
		header.Size = rng.Int63n(0o77777777777 + 1)
	case 1:
		header.Typeflag = tar.TypeDir
	case 2:
		header.Typeflag = tar.TypeSymlink
		header.Linkname = randomString(rng, 1+rng.Intn(100))
	}
	return header
}

func randomString(rng *rand.Rand, length int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-"
	var builder strings.Builder
	for i := 0; i < length; i++ {
		builder.WriteByte(alphabet[rng.Intn(len(alphabet))])
	}
	return builder.String()
}

package tarreader

import "testing"

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

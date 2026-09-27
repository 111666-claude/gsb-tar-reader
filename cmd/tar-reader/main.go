// Command tar-reader 造一个 ustar 头再解析它。
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/tarreader"
)

// Run 执行一次命令行调用，返回退出码。
func Run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("tar-reader", flag.ContinueOnError)
	flags.SetOutput(stderr)
	name := flags.String("name", "file.txt", "name 字段")
	prefix := flags.String("prefix", "", "prefix 字段（ustar 长名）")
	size := flags.Int64("size", 42, "文件大小")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	block := tarreader.BuildHeader(*name, *size, *prefix)
	header, err := tarreader.Parse(block)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "name=%s size=%d mode=%o type=%c checksum=%d\n", header.Name, header.Size, header.Mode, header.Typeflag, header.Checksum)
	return 0
}

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

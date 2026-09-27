# tar-reader

解析 ustar 头（512 字节块、校验和、长名前缀），只用 Go 标准库。

```
go test ./...
go run ./cmd/tar-reader --name file.txt
go run ./cmd/tar-reader --name long.txt --prefix deep/nested/path
```

## 口径（README 为准）

- **校验和要把 chksum 字段当成 8 个空格再求和**（这是 ustar 的规定，`archive/tar` 也这么做）。
  现在把该字段当数字算，所以自己造出来的合法头也报错：
  `--name file.txt` 报 `校验和错误：算出 4835，头部写着 4762`，按口径必须正常解析。
- **长名要拼 prefix**：`name` 是 100 字节、`prefix` 是 155 字节，完整名字是 `prefix + "/" + name`
  （prefix 为空时只用 name）；现在只看 name，长名会被截断成 `long.txt`。
- 数字字段（mode/uid/gid/size/mtime）是八进制 ASCII、以 NUL 或空格结尾；解析失败要报错并指出字段名。
- 类型标志要认：`0`/`\x00` 普通文件、`5` 目录、`2` 符号链接（linkname 里是目标）；未知标志报错。
- 不变量：`Parse(BuildHeader(name, size, prefix))` 给出的 name/size/prefix 与输入一致；
  校验和用规范算法重算一致；同一块重复解析结果相同。
- 规模：100 万个头（512 MB）单遍解析，内存 O(1)（不把整份归档读进内存）。

## 现在的行为

- 校验和按规范把 chksum 字段当 8 个空格求和，`--name file.txt` 正常解析。
- 长名拼 `prefix + "/" + name`（prefix 为空时只用 name）。
- 数字字段按八进制解析，失败报错并指出字段名；size 为负报错。
- 类型标志认 `0`/`\x00`（普通文件）、`5`（目录）、`2`（符号链接），未知标志报错。

## 计划新增的能力

```
--name / --prefix / --size    造一个 ustar 头再解析
```

## 输出

```
name=… size=… mode=… type=… checksum=…
```

## 目录

```
tar.go           头解析与构造
cmd/tar-reader   命令行入口
tar_test.go      go test 用例
```

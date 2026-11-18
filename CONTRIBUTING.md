# 贡献指南

本项目使用 Go 1.25 或更高版本。提交修改前，从仓库根目录运行完整回归：

```sh
go test -count=1 ./...
```

GitHub Actions 的 `regression` 检查运行同一条命令。涉及 HTTP 行为的变更，建议在 `main_test.go` 补充相应的请求与响应断言。

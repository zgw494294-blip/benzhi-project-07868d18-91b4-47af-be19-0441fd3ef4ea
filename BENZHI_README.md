# BENZHI_README

基于 Go 实现的cleanroom-monitor-release HTTP API 项目，一款后端服务，面向洁净室环境监测团队的合规放行服务，完整实现方案建档、校准就绪、现场采样、自动发现、人工裁决、整改补测、证据冻结、凭据签发与审计追溯。

## 项目说明
- 项目：benzhi-project-07868d18-91b4-47af-be19-0441fd3ef4ea
- 项目用途：面向洁净室环境监测团队的合规放行服务，完整实现方案建档、校准就绪、现场采样、自动发现、人工裁决、整改补测、证据冻结、凭据签发与审计追溯。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server -selfcheck -addr=127.0.0.1:19091
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-07868d18-91b4-47af-be19-0441fd3ef4ea-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-07868d18-91b4-47af-be19-0441fd3ef4ea-arm64 linux/arm64
docker run -it benzhi-project-07868d18-91b4-47af-be19-0441fd3ef4ea-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -selfcheck -addr=127.0.0.1:19091`

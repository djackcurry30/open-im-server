// Copyright © 2023 OpenIM. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main 是 OpenIM API 服务的入口程序。
//
// API 服务提供 RESTful HTTP 接口，是客户端与 OpenIM 后端服务交互的主要入口。
// 该服务负责：
//   - 接收和处理客户端的 HTTP 请求
//   - 调用后端 RPC 服务完成业务逻辑
//   - 返回 JSON 格式的响应数据
//   - 提供 API 文档和健康检查接口
//
// 主要功能模块：
//   - 用户认证和授权
//   - 用户管理
//   - 好友关系管理
//   - 群组管理
//   - 消息发送和查询
//   - 会话管理
//   - 第三方服务集成
//
// 使用方法：
//
//	./openim-api
//
// 注意：该服务依赖配置文件和后端 RPC 服务的正常运行。
package main

import (
	_ "net/http/pprof" // 导入 pprof 用于性能分析

	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是 API 服务的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建 API 命令对象
//  2. 执行命令，启动 HTTP 服务器
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewApiCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

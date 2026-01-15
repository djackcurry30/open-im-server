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

// Package main 是 OpenIM 消息网关服务的入口程序。
//
// 消息网关是 OpenIM 的核心组件之一，负责管理客户端的 WebSocket 长连接。
// 该服务的主要职责：
//   - 接受客户端的 WebSocket 连接请求
//   - 验证客户端的认证 Token
//   - 维护在线用户的连接状态
//   - 实现多端登录策略（不踢、同类型踢、全踢等）
//   - 接收来自客户端的消息并转发到消息服务
//   - 将服务端消息推送到在线客户端
//   - 处理心跳保活机制
//
// 架构特点：
//   - 支持水平扩展，可部署多个实例
//   - 通过服务发现机制实现负载均衡
//   - 使用 Redis 存储在线用户的连接信息
//
// 使用方法：
//
//	./openim-msggateway
//
// 注意：该服务需要配置 WebSocket 监听端口和 RPC 服务地址。
package main

import (
	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是消息网关服务的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建消息网关命令对象
//  2. 执行命令，启动 WebSocket 服务器
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewMsgGatewayCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

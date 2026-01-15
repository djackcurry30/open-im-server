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

// Package main 是 OpenIM 消息 RPC 服务的入口程序。
//
// 消息服务是 OpenIM 的核心服务，负责处理所有消息相关的业务逻辑。
// 该服务的主要职责：
//   - 接收和验证客户端发送的消息
//   - 执行消息拦截器链（敏感词过滤、消息转换等）
//   - 生成消息序列号
//   - 将消息写入 Kafka 消息队列
//   - 处理消息撤回、删除、已读等操作
//   - 提供消息查询和同步接口
//   - 管理消息的生命周期
//
// 消息处理流程：
//  1. 验证消息格式和权限
//  2. 执行消息拦截器（如敏感词过滤）
//  3. 生成全局唯一的消息序列号
//  4. 将消息发送到 Kafka 队列
//  5. 返回发送结果给客户端
//
// 架构特点：
//   - 作为 gRPC 服务提供消息处理接口
//   - 支持水平扩展，可部署多个实例
//   - 通过 Kafka 实现消息的异步处理
//   - 通过服务发现机制注册到注册中心
//
// 使用方法：
//
//	./openim-rpc-msg
//
// 注意：该服务依赖 Kafka、MongoDB 和 Redis 的正常运行。
package main

import (
	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是消息 RPC 服务的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建消息 RPC 命令对象
//  2. 执行命令，启动 gRPC 服务器
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewMsgRpcCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

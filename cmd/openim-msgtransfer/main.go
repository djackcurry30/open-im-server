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

// Package main 是 OpenIM 消息传输服务的入口程序。
//
// 消息传输服务负责处理消息在系统中的流转和持久化。
// 该服务的主要职责：
//   - 从 Kafka 消息队列消费消息
//   - 将消息持久化到 MongoDB 数据库
//   - 将消息推送到在线用户（通过消息网关）
//   - 触发离线推送（通过推送服务）
//   - 更新会话的最新消息信息
//   - 更新 Redis 缓存中的消息序列号
//
// 消息处理流程：
//  1. 从 Kafka 的 toMongo 主题消费消息
//  2. 批量写入 MongoDB 进行持久化
//  3. 从 Kafka 的 toPush 主题消费消息
//  4. 推送消息到在线用户或触发离线推送
//
// 架构特点：
//   - 支持多实例部署，通过 Kafka 分区实现负载均衡
//   - 批量处理消息，提高吞吐量
//   - 异步处理，不阻塞消息发送流程
//
// 使用方法：
//
//	./openim-msgtransfer
//
// 注意：该服务依赖 Kafka、MongoDB 和 Redis 的正常运行。
package main

import (
	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是消息传输服务的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建消息传输命令对象
//  2. 执行命令，启动 Kafka 消费者
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewMsgTransferCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

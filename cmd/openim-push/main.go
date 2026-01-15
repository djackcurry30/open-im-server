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

// Package main 是 OpenIM 推送服务的入口程序。
//
// 推送服务负责将消息推送到在线和离线用户。
// 该服务的主要职责：
//   - 接收推送请求（来自消息传输服务）
//   - 判断用户是否在线
//   - 对于在线用户，通过消息网关推送消息
//   - 对于离线用户，调用第三方推送服务发送通知
//   - 支持多种第三方推送平台（FCM、APNs、华为、小米等）
//   - 处理推送失败重试逻辑
//
// 推送策略：
//   - 优先推送到在线设备
//   - 离线设备根据配置选择推送平台
//   - 支持推送消息的去重和合并
//   - 支持推送消息的优先级设置
//
// 架构特点：
//   - 作为 RPC 服务提供推送接口
//   - 支持多种推送渠道的统一管理
//   - 可配置推送策略和失败重试机制
//
// 使用方法：
//
//	./openim-push
//
// 注意：该服务需要配置第三方推送平台的凭证信息。
package main

import (
	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是推送服务的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建推送 RPC 命令对象
//  2. 执行命令，启动 gRPC 服务器
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewPushRpcCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

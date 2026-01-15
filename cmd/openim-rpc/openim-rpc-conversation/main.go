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

// Package main 是 OpenIM 会话 RPC 服务的入口程序。
//
// 会话服务负责管理用户的聊天会话。
// 该服务的主要职责：
//   - 创建和管理单聊、群聊会话
//   - 获取用户的会话列表
//   - 更新会话的最新消息和未读数
//   - 设置会话属性（置顶、免打扰等）
//   - 删除和清空会话
//   - 会话的排序和过滤
//
// 会话管理：
//   - 每个用户与其他用户或群组的聊天都对应一个会话
//   - 会话包含最新消息、未读数、置顶状态等信息
//   - 支持会话的批量操作
//   - 会话数据存储在 MongoDB，热点数据缓存在 Redis
//
// 架构特点：
//   - 作为 gRPC 服务提供会话管理接口
//   - 支持水平扩展
//   - 通过服务发现机制注册到注册中心
//
// 使用方法：
//
//	./openim-rpc-conversation
//
// 注意：该服务依赖 MongoDB 和 Redis 的正常运行。
package main

import (
	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是会话 RPC 服务的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建会话 RPC 命令对象
//  2. 执行命令，启动 gRPC 服务器
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewConversationRpcCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

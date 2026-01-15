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

// Package main 是 OpenIM 好友关系 RPC 服务的入口程序。
//
// 好友关系服务负责管理用户之间的好友关系和黑名单。
// 该服务的主要职责：
//   - 处理好友申请和审批
//   - 添加和删除好友
//   - 获取好友列表
//   - 管理黑名单
//   - 设置好友备注和分组
//   - 检查好友关系状态
//
// 关系管理：
//   - 好友关系是双向的，需要双方确认
//   - 支持好友申请的同意和拒绝
//   - 黑名单功能可以屏蔽特定用户的消息
//   - 好友数据存储在 MongoDB，热点数据缓存在 Redis
//
// 架构特点：
//   - 作为 gRPC 服务提供好友关系管理接口
//   - 支持水平扩展
//   - 通过服务发现机制注册到注册中心
//
// 使用方法：
//
//	./openim-rpc-friend
//
// 注意：该服务依赖 MongoDB 和 Redis 的正常运行。
package main

import (
	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是好友关系 RPC 服务的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建好友关系 RPC 命令对象
//  2. 执行命令，启动 gRPC 服务器
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewFriendRpcCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

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

// Package main 是 OpenIM 群组 RPC 服务的入口程序。
//
// 群组服务负责管理群组和群成员。
// 该服务的主要职责：
//   - 创建和解散群组
//   - 管理群成员（邀请、踢出、退出）
//   - 设置群组信息（名称、头像、公告等）
//   - 管理群成员角色（群主、管理员、普通成员）
//   - 处理入群申请和审批
//   - 设置群组属性（全员禁言、入群验证等）
//   - 获取群组列表和群成员列表
//
// 群组管理：
//   - 支持多种群组类型（普通群、超级群等）
//   - 群主可以转让群组
//   - 管理员可以管理群成员和群设置
//   - 群组数据存储在 MongoDB，热点数据缓存在 Redis
//
// 架构特点：
//   - 作为 gRPC 服务提供群组管理接口
//   - 支持水平扩展
//   - 通过服务发现机制注册到注册中心
//
// 使用方法：
//
//	./openim-rpc-group
//
// 注意：该服务依赖 MongoDB 和 Redis 的正常运行。
package main

import (
	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是群组 RPC 服务的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建群组 RPC 命令对象
//  2. 执行命令，启动 gRPC 服务器
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewGroupRpcCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

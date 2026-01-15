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

// Package main 是 OpenIM 用户 RPC 服务的入口程序。
//
// 用户服务负责管理用户信息和状态。
// 该服务的主要职责：
//   - 用户注册和信息管理
//   - 获取和更新用户资料（昵称、头像、个性签名等）
//   - 用户状态管理（在线、离线、忙碌等）
//   - 用户搜索和查询
//   - 批量获取用户信息
//   - 用户隐私设置
//   - 用户账号管理（禁用、注销等）
//
// 用户管理：
//   - 用户基本信息存储在 MongoDB
//   - 用户在线状态存储在 Redis
//   - 热点用户数据缓存在 Redis
//   - 支持用户信息的批量查询和更新
//
// 架构特点：
//   - 作为 gRPC 服务提供用户管理接口
//   - 支持水平扩展
//   - 通过服务发现机制注册到注册中心
//   - 使用缓存提高查询性能
//
// 使用方法：
//
//	./openim-rpc-user
//
// 注意：该服务依赖 MongoDB 和 Redis 的正常运行。
package main

import (
	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是用户 RPC 服务的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建用户 RPC 命令对象
//  2. 执行命令，启动 gRPC 服务器
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewUserRpcCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

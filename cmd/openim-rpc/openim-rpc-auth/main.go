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

// Package main 是 OpenIM 认证 RPC 服务的入口程序。
//
// 认证服务负责用户身份验证和授权管理。
// 该服务的主要职责：
//   - 用户登录认证，生成访问 Token
//   - Token 的验证和解析
//   - Token 的刷新和失效管理
//   - 多端登录策略控制
//   - 用户权限验证
//   - 管理员权限管理
//
// Token 管理：
//   - 使用 JWT 生成和验证 Token
//   - Token 包含用户 ID、平台 ID 等信息
//   - 支持 Token 的主动失效（如强制下线）
//   - 在 Redis 中维护 Token 的有效性
//
// 架构特点：
//   - 作为 gRPC 服务提供认证接口
//   - 支持水平扩展
//   - 通过服务发现机制注册到注册中心
//
// 使用方法：
//
//	./openim-rpc-auth
//
// 注意：该服务需要配置 JWT 密钥和 Token 过期时间。
package main

import (
	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是认证 RPC 服务的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建认证 RPC 命令对象
//  2. 执行命令，启动 gRPC 服务器
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewAuthRpcCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

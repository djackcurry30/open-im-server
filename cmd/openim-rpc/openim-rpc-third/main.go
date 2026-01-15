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

// Package main 是 OpenIM 第三方服务 RPC 的入口程序。
//
// 第三方服务负责集成外部服务和资源管理。
// 该服务的主要职责：
//   - 对象存储管理（MinIO、阿里云 OSS、腾讯云 COS、AWS S3 等）
//   - 文件上传和下载
//   - 生成文件访问 URL
//   - 管理文件的生命周期
//   - 提供文件元数据查询
//   - 集成其他第三方服务（如短信、邮件等）
//
// 对象存储：
//   - 支持多种对象存储后端
//   - 统一的文件上传下载接口
//   - 支持分片上传大文件
//   - 自动生成带签名的访问 URL
//   - 文件元数据存储在 MongoDB
//
// 架构特点：
//   - 作为 gRPC 服务提供第三方服务接口
//   - 支持水平扩展
//   - 通过服务发现机制注册到注册中心
//   - 支持多种存储后端的灵活切换
//
// 使用方法：
//
//	./openim-rpc-third
//
// 注意：该服务需要配置对象存储的访问凭证和端点信息。
package main

import (
	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是第三方服务 RPC 的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建第三方服务 RPC 命令对象
//  2. 执行命令，启动 gRPC 服务器
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewThirdRpcCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

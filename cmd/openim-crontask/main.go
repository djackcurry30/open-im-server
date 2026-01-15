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

// Package main 是 OpenIM 定时任务服务的入口程序。
//
// 定时任务服务负责执行系统的后台定时任务。
// 该服务的主要职责：
//   - 定期清理过期数据（如过期的消息、日志等）
//   - 定期统计和汇总数据
//   - 定期检查系统健康状态
//   - 执行数据备份和归档任务
//   - 处理延迟任务和定时通知
//
// 任务调度：
//   - 使用 cron 表达式配置任务执行时间
//   - 支持任务的启用和禁用
//   - 记录任务执行历史和结果
//   - 任务执行失败时支持告警通知
//
// 架构特点：
//   - 独立的后台服务，不影响主业务流程
//   - 支持分布式部署，通过分布式锁避免重复执行
//   - 可配置任务的执行频率和超时时间
//
// 使用方法：
//
//	./openim-crontask
//
// 注意：该服务需要配置定时任务的执行计划和相关参数。
package main

import (
	"github.com/openimsdk/open-im-server/v3/pkg/common/cmd"
	"github.com/openimsdk/tools/system/program"
)

// main 是定时任务服务的主入口函数。
//
// 该函数执行以下操作：
//  1. 创建定时任务命令对象
//  2. 执行命令，启动任务调度器
//  3. 如果启动失败，记录错误并退出程序
//
// 退出码：
//   - 0: 正常退出
//   - 非 0: 启动失败或运行时错误
func main() {
	if err := cmd.NewCronTaskCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}

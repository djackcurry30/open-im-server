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

// Package api 提供 OpenIM 的 HTTP API 服务实现。
//
// 该包负责初始化和管理 HTTP 服务器，处理客户端的 REST API 请求。
// 主要功能包括：
//   - HTTP 服务器的启动和配置
//   - 路由注册和请求分发
//   - 优雅关闭机制
//   - 服务健康检查
package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	conf "github.com/openimsdk/open-im-server/v3/pkg/common/config"
	"github.com/openimsdk/tools/discovery"
	"github.com/openimsdk/tools/log"
	"github.com/openimsdk/tools/utils/datautil"
	"github.com/openimsdk/tools/utils/network"
	"github.com/openimsdk/tools/utils/runtimeenv"
	"google.golang.org/grpc"
)

// Config 是 API 服务的配置结构体。
//
// 该结构体包含 API 服务运行所需的所有配置信息。
type Config struct {
	conf.AllConfig // 全局配置，包含所有服务的配置

	ConfigPath conf.Path  // 配置文件路径
	Index      conf.Index // 服务实例索引，用于多实例部署时区分不同实例
}

// Start 启动 API HTTP 服务器。
//
// 该函数是 API 服务的主入口，负责完成以下工作：
//  1. 根据配置获取 API 服务的监听端口
//  2. 初始化 Gin 路由器，注册所有 HTTP 路由
//  3. 创建并启动 HTTP 服务器
//  4. 监听上下文取消信号，实现优雅关闭
//  5. 等待服务器完全关闭后返回
//
// 优雅关闭流程：
//   - 接收到取消信号后，停止接受新的连接
//   - 等待现有请求处理完成（最多 15 秒）
//   - 关闭服务器并释放资源
//
// 参数：
//   - ctx: 上下文对象，用于控制服务生命周期
//   - config: API 服务配置
//   - client: 服务发现注册客户端
//   - service: gRPC 服务注册器（当前未使用）
//
// 返回值：
//   - error: 服务退出原因，正常退出时返回 "api done" 错误
func Start(ctx context.Context, config *Config, client discovery.SvcDiscoveryRegistry, service grpc.ServiceRegistrar) error {
	// 根据实例索引获取 API 服务的监听端口
	apiPort, err := datautil.GetElemByIndex(config.API.Api.Ports, int(config.Index))
	if err != nil {
		return err
	}

	// 初始化 Gin 路由器，注册所有 HTTP 路由和中间件
	router, err := newGinRouter(ctx, client, config)
	if err != nil {
		return err
	}

	// 创建 API 服务的上下文，用于控制服务生命周期
	apiCtx, apiCancel := context.WithCancelCause(context.Background())
	done := make(chan struct{}) // 用于通知服务器已完全关闭

	go func() {
		// 创建 HTTP 服务器
		httpServer := &http.Server{
			Handler: router,
			Addr:    net.JoinHostPort(network.GetListenIP(config.API.Api.ListenIP), strconv.Itoa(apiPort)),
		}

		// 启动优雅关闭监听器
		go func() {
			defer close(done)
			select {
			case <-ctx.Done():
				// 主上下文取消，记录原因并触发关闭
				apiCancel(fmt.Errorf("recv ctx %w", context.Cause(ctx)))
			case <-apiCtx.Done():
				// API 上下文取消（服务器自身退出）
			}
			log.ZDebug(ctx, "api server is shutting down")
			// 优雅关闭 HTTP 服务器
			if err := httpServer.Shutdown(context.Background()); err != nil {
				log.ZWarn(ctx, "api server shutdown err", err)
			}
		}()

		// 记录服务器启动信息
		log.CInfo(ctx, "api server is init", "runtimeEnv", runtimeenv.RuntimeEnvironment(), "address", httpServer.Addr, "apiPort", apiPort)

		// 启动 HTTP 服务器（阻塞直到服务器关闭）
		err := httpServer.ListenAndServe()
		if err == nil {
			// 正常退出时设置退出原因
			err = errors.New("api done")
		}
		apiCancel(err)
	}()

	// 以下代码用于配置热更新（当前已注释）
	// 如果启用 ETCD 服务发现，可以监听配置变化并自动更新
	//if config.Discovery.Enable == conf.ETCD {
	//	cm := disetcd.NewConfigManager(client.(*etcd.SvcDiscoveryRegistryImpl).GetClient(), config.GetConfigNames())
	//	cm.Watch(ctx)
	//}

	// 以下代码用于信号处理（当前已注释）
	// 可以监听系统信号（如 SIGTERM）来触发优雅关闭
	//sigs := make(chan os.Signal, 1)
	//signal.Notify(sigs, syscall.SIGTERM)
	//select {
	//case val := <-sigs:
	//	log.ZDebug(ctx, "recv exit", "signal", val.String())
	//	cancel(fmt.Errorf("signal %s", val.String()))
	//case <-ctx.Done():
	//}

	// 等待 API 服务器退出
	<-apiCtx.Done()
	exitCause := context.Cause(apiCtx)
	log.ZWarn(ctx, "api server exit", exitCause)

	// 等待服务器完全关闭，最多等待 15 秒
	timer := time.NewTimer(time.Second * 15)
	defer timer.Stop()
	select {
	case <-timer.C:
		// 超时，强制退出
		log.ZWarn(ctx, "api server graceful stop timeout", nil)
	case <-done:
		// 服务器已完全关闭
		log.ZDebug(ctx, "api server graceful stop done")
	}
	return exitCause
}

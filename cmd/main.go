// Package main 是 OpenIM 服务器的主入口程序。
//
// 本程序负责启动和管理 OpenIM 即时通讯服务器的所有微服务组件，包括：
//   - RPC 服务：认证、用户、群组、消息、好友、会话、第三方服务
//   - 消息网关：处理客户端 WebSocket 长连接
//   - 消息传输：处理消息队列和持久化
//   - 推送服务：处理离线消息推送
//   - API 服务：提供 REST API 接口
//   - 定时任务：执行后台定时任务
//
// 主要功能：
//   - 解析命令行参数，加载配置文件
//   - 初始化所有服务的配置
//   - 按照依赖关系启动各个微服务
//   - 管理服务生命周期
//   - 处理系统信号，实现优雅关闭
//
// 使用方法：
//
//	./openim-server -c /path/to/config
//
// 其中 -c 参数指定配置文件目录路径。
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/openimsdk/open-im-server/v3/internal/api"
	"github.com/openimsdk/open-im-server/v3/internal/msggateway"
	"github.com/openimsdk/open-im-server/v3/internal/msgtransfer"
	"github.com/openimsdk/open-im-server/v3/internal/push"
	"github.com/openimsdk/open-im-server/v3/internal/rpc/auth"
	"github.com/openimsdk/open-im-server/v3/internal/rpc/conversation"
	"github.com/openimsdk/open-im-server/v3/internal/rpc/group"
	"github.com/openimsdk/open-im-server/v3/internal/rpc/msg"
	"github.com/openimsdk/open-im-server/v3/internal/rpc/relation"
	"github.com/openimsdk/open-im-server/v3/internal/rpc/third"
	"github.com/openimsdk/open-im-server/v3/internal/rpc/user"
	"github.com/openimsdk/open-im-server/v3/internal/tools/cron"
	"github.com/openimsdk/open-im-server/v3/pkg/common/config"
	"github.com/openimsdk/open-im-server/v3/pkg/common/prommetrics"
	"github.com/openimsdk/open-im-server/v3/version"
	"github.com/openimsdk/tools/discovery"
	"github.com/openimsdk/tools/discovery/standalone"
	"github.com/openimsdk/tools/log"
	"github.com/openimsdk/tools/system/program"
	"github.com/openimsdk/tools/utils/datautil"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

// init 初始化全局配置和监控指标。
//
// 该函数在程序启动时自动执行，完成以下初始化工作：
//   - 设置为单机模式（standalone）
//   - 注册所有 Prometheus 监控指标
func init() {
	config.SetStandalone()
	prommetrics.RegistryAll()
}

// main 是程序的主入口函数。
//
// 主要执行流程：
//  1. 解析命令行参数，获取配置文件路径
//  2. 创建命令管理器，加载配置
//  3. 按顺序注册所有微服务：
//     - 非阻塞服务（RPC 服务）：auth、conversation、relation、group、msg、third、user、push
//     - 阻塞服务：msggateway、msgtransfer、api、cron
//  4. 启动所有服务并管理其生命周期
//  5. 等待服务退出或接收到终止信号
//
// 命令行参数：
//   - -c: 配置文件目录路径（必需）
//
// 退出码：
//   - 0: 正常退出
//   - 1: 配置路径为空或服务启动失败
func main() {
	var configPath string
	// 解析命令行参数
	flag.StringVar(&configPath, "c", "", "config path")
	flag.Parse()

	// 验证配置路径是否提供
	if configPath == "" {
		_, _ = fmt.Fprintln(os.Stderr, "config path is empty")
		os.Exit(1)
		return
	}

	// 创建命令管理器
	cmd := newCmds(configPath)

	// 注册 RPC 服务（非阻塞模式）
	// 这些服务会在后台运行，不会阻塞主流程
	putCmd(cmd, false, auth.Start)         // 认证服务
	putCmd(cmd, false, conversation.Start) // 会话服务
	putCmd(cmd, false, relation.Start)     // 好友关系服务
	putCmd(cmd, false, group.Start)        // 群组服务
	putCmd(cmd, false, msg.Start)          // 消息服务
	putCmd(cmd, false, third.Start)        // 第三方服务
	putCmd(cmd, false, user.Start)         // 用户服务
	putCmd(cmd, false, push.Start)         // 推送服务

	// 注册阻塞服务
	// 这些服务需要持续运行，会阻塞主流程
	putCmd(cmd, true, msggateway.Start)  // 消息网关
	putCmd(cmd, true, msgtransfer.Start) // 消息传输
	putCmd(cmd, true, api.Start)         // API 服务
	putCmd(cmd, true, cron.Start)        // 定时任务

	// 启动所有服务
	ctx := context.Background()
	if err := cmd.run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "server exit %s", err)
		os.Exit(1)
		return
	}
}

// newCmds 创建一个新的命令管理器实例。
//
// 参数：
//   - confPath: 配置文件目录路径
//
// 返回值：
//   - *cmds: 命令管理器实例
func newCmds(confPath string) *cmds {
	return &cmds{confPath: confPath}
}

// cmdName 表示一个服务命令的配置信息。
type cmdName struct {
	Name  string                          // 服务名称
	Func  func(ctx context.Context) error // 服务启动函数
	Block bool                            // 是否为阻塞服务
}

// cmds 是命令管理器，负责管理所有微服务的配置和启动。
type cmds struct {
	confPath string                   // 配置文件目录路径
	cmds     []cmdName                // 所有注册的服务命令列表
	config   config.AllConfig         // 全局配置对象
	conf     map[string]reflect.Value // 配置类型路径到配置值的映射表
}

// getTypePath 获取类型的完整路径。
//
// 返回格式为 "包路径/类型名"，用于唯一标识一个配置类型。
//
// 参数：
//   - typ: 反射类型对象
//
// 返回值：
//   - string: 类型的完整路径
func (x *cmds) getTypePath(typ reflect.Type) string {
	return path.Join(typ.PkgPath(), typ.Name())
}

// initDiscovery 初始化服务发现配置。
//
// 该方法将服务发现模式设置为 standalone（单机模式），
// 并自动配置所有 RPC 服务的服务名称。
func (x *cmds) initDiscovery() {
	x.config.Discovery.Enable = "standalone"
	vof := reflect.ValueOf(&x.config.Discovery.RpcService).Elem()
	tof := reflect.TypeOf(&x.config.Discovery.RpcService).Elem()
	num := tof.NumField()
	// 遍历所有 RPC 服务字段，将字段名设置为服务名
	for i := 0; i < num; i++ {
		field := tof.Field(i)
		if !field.IsExported() {
			continue
		}
		if field.Type.Kind() != reflect.String {
			continue
		}
		vof.Field(i).SetString(field.Name)
	}
}

// initAllConfig 初始化所有服务的配置。
//
// 该方法执行以下操作：
//  1. 遍历全局配置对象的所有字段
//  2. 为每个配置类型建立类型路径到配置值的映射
//  3. 从配置文件目录读取对应的 YAML 配置文件
//  4. 使用 Viper 解析 YAML 并反序列化到配置结构体
//  5. 初始化服务发现、Redis、本地缓存和通知配置
//
// 返回值：
//   - error: 配置初始化失败时返回错误
func (x *cmds) initAllConfig() error {
	x.conf = make(map[string]reflect.Value)
	vof := reflect.ValueOf(&x.config).Elem()
	num := vof.NumField()

	// 遍历全局配置的所有字段
	for i := 0; i < num; i++ {
		field := vof.Field(i)
		// 解引用指针类型，获取实际的配置对象
		for ptr := true; ptr; {
			if field.Kind() == reflect.Ptr {
				field = field.Elem()
			} else {
				ptr = false
			}
		}
		// 建立类型路径到配置值的映射
		x.conf[x.getTypePath(field.Type())] = field
		val := field.Addr().Interface()

		// 获取配置文件名
		name := val.(interface{ GetConfigFileName() string }).GetConfigFileName()

		// 读取配置文件
		confData, err := os.ReadFile(filepath.Join(x.confPath, name))
		if err != nil {
			if os.IsNotExist(err) {
				// 配置文件不存在时跳过（某些配置可能是可选的）
				continue
			}
			return err
		}

		// 使用 Viper 解析 YAML 配置
		v := viper.New()
		v.SetConfigType("yaml")
		if err := v.ReadConfig(bytes.NewReader(confData)); err != nil {
			return err
		}

		// 反序列化配置到结构体
		opt := func(conf *mapstructure.DecoderConfig) {
			conf.TagName = config.StructTagName
		}
		if err := v.Unmarshal(val, opt); err != nil {
			return err
		}
	}

	// 初始化服务发现配置
	x.initDiscovery()

	// 配置 Redis 和本地缓存
	x.config.Redis.Disable = false
	x.config.LocalCache = config.LocalCache{}

	// 初始化通知配置
	config.InitNotification(&x.config.Notification)

	return nil
}

// parseConf 解析并填充服务配置对象。
//
// 该方法通过反射机制，将全局配置中的各个子配置注入到服务的配置结构体中。
// 支持以下特殊配置类型：
//   - config.Index: 索引配置（跳过）
//   - config.Path: 配置文件路径
//   - config.AllConfig: 全局配置对象
//
// 参数：
//   - conf: 服务配置对象（指针类型）
//
// 返回值：
//   - error: 配置解析失败时返回错误
func (x *cmds) parseConf(conf any) error {
	vof := reflect.ValueOf(conf)
	// 解引用指针，获取实际的配置对象
	for {
		if vof.Kind() == reflect.Ptr {
			vof = vof.Elem()
		} else {
			break
		}
	}
	tof := vof.Type()
	numField := vof.NumField()

	// 遍历配置对象的所有字段
	for i := 0; i < numField; i++ {
		typeField := tof.Field(i)
		if !typeField.IsExported() {
			continue
		}
		field := vof.Field(i)
		pkt := x.getTypePath(field.Type())
		val, ok := x.conf[pkt]

		if !ok {
			// 处理特殊配置类型
			switch field.Interface().(type) {
			case config.Index:
				// 索引配置，跳过
			case config.Path:
				// 设置配置文件路径
				field.SetString(x.confPath)
			case config.AllConfig:
				// 注入全局配置对象
				field.Set(reflect.ValueOf(x.config))
			case *config.AllConfig:
				// 注入全局配置对象指针
				field.Set(reflect.ValueOf(&x.config))
			default:
				// 未找到对应的配置，返回错误
				return fmt.Errorf("config field %s %s not found", vof.Type().Name(), typeField.Name)
			}
			continue
		}
		// 注入配置值
		field.Set(val)
	}
	return nil
}

// add 向命令管理器添加一个服务命令。
//
// 参数：
//   - name: 服务名称
//   - block: 是否为阻塞服务
//   - fn: 服务启动函数
func (x *cmds) add(name string, block bool, fn func(ctx context.Context) error) {
	x.cmds = append(x.cmds, cmdName{Name: name, Block: block, Func: fn})
}

// initLog 初始化日志系统。
//
// 根据全局配置初始化日志记录器，配置包括：
//   - 日志级别
//   - 输出目标（标准输出/文件）
//   - 日志格式（JSON/文本）
//   - 日志存储位置
//   - 日志轮转策略
//
// 返回值：
//   - error: 日志初始化失败时返回错误
func (x *cmds) initLog() error {
	conf := x.config.Log
	if err := log.InitLoggerFromConfig(
		"openim-server",
		program.GetProcessName(),
		"", "",
		conf.RemainLogLevel,
		conf.IsStdout,
		conf.IsJson,
		conf.StorageLocation,
		conf.RemainRotationCount,
		conf.RotationTime,
		strings.TrimSpace(version.Version),
		conf.IsSimplify,
	); err != nil {
		return err
	}
	return nil

}

// run 启动所有注册的服务并管理其生命周期。
//
// 该方法执行以下操作：
//  1. 初始化所有服务配置
//  2. 初始化日志系统
//  3. 创建可取消的上下文，用于控制服务生命周期
//  4. 启动 Prometheus 监控服务（如果启用）
//  5. 注册系统信号处理器（SIGTERM、SIGINT）
//  6. 启动所有非阻塞服务（RPC 服务）
//  7. 启动所有阻塞服务（网关、传输、API 等）
//  8. 等待服务退出或接收到终止信号
//  9. 执行优雅关闭，等待所有服务完成清理工作
//
// 参数：
//   - ctx: 上下文对象
//
// 返回值：
//   - error: 服务启动或运行失败时返回错误
func (x *cmds) run(ctx context.Context) error {
	if len(x.cmds) == 0 {
		return fmt.Errorf("no command to run")
	}

	// 初始化所有配置
	if err := x.initAllConfig(); err != nil {
		return err
	}

	// 初始化日志系统
	if err := x.initLog(); err != nil {
		return err
	}

	// 创建可取消的上下文，用于控制服务生命周期
	ctx, cancel := context.WithCancelCause(ctx)

	// 监听上下文取消事件，记录退出原因
	go func() {
		<-ctx.Done()
		log.ZError(ctx, "context server exit cause", context.Cause(ctx))
	}()

	// 启动 Prometheus 监控服务
	if prometheus := x.config.API.Prometheus; prometheus.Enable {
		var (
			port int
			err  error
		)
		// 获取 Prometheus 监听端口
		if !prometheus.AutoSetPorts {
			port, err = datautil.GetElemByIndex(prometheus.Ports, 0)
			if err != nil {
				return err
			}
		}
		// 创建 TCP 监听器
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return fmt.Errorf("prometheus listen %d error %w", port, err)
		}
		defer listener.Close()
		log.ZDebug(ctx, "prometheus start", "addr", listener.Addr())

		// 在后台启动 Prometheus HTTP 服务
		go func() {
			err := prommetrics.Start(listener)
			if err == nil {
				err = fmt.Errorf("http done")
			}
			cancel(fmt.Errorf("prometheus %w", err))
		}()
	}

	// 注册系统信号处理器，实现优雅关闭
	go func() {
		sigs := make(chan os.Signal, 1)
		// 监听 SIGTERM 和 SIGINT 信号
		signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
		select {
		case <-ctx.Done():
			return
		case val := <-sigs:
			log.ZDebug(ctx, "recv signal", "signal", val.String())
			cancel(fmt.Errorf("signal %s", val.String()))
		}
	}()

	// 启动所有非阻塞服务（RPC 服务）
	for i := range x.cmds {
		cmd := x.cmds[i]
		if cmd.Block {
			continue
		}
		if err := cmd.Func(ctx); err != nil {
			cancel(fmt.Errorf("server %s exit %w", cmd.Name, err))
			return err
		}
		go func() {
			if cmd.Block {
				cancel(fmt.Errorf("server %s exit", cmd.Name))
			}
		}()
	}

	// 启动所有阻塞服务并管理其生命周期
	var wait cmdManger
	for i := range x.cmds {
		cmd := x.cmds[i]
		if !cmd.Block {
			continue
		}
		wait.Start(cmd.Name)
		go func() {
			defer wait.Shutdown(cmd.Name)
			if err := cmd.Func(ctx); err != nil {
				cancel(fmt.Errorf("server %s exit %w", cmd.Name, err))
				return
			}
			cancel(fmt.Errorf("server %s exit", cmd.Name))
		}()
	}

	// 等待上下文取消（服务退出或接收到信号）
	<-ctx.Done()
	exitCause := context.Cause(ctx)
	log.ZWarn(ctx, "notification of service closure", exitCause)

	// 等待所有服务完成清理工作（优雅关闭）
	done := wait.Wait()
	timeout := time.NewTimer(time.Second * 10)
	defer timeout.Stop()

	for {
		select {
		case <-timeout.C:
			// 超时，强制退出
			log.ZWarn(ctx, "server exit timeout", nil, "running", wait.Running())
			return exitCause
		case _, ok := <-done:
			if ok {
				// 仍有服务在运行
				log.ZWarn(ctx, "waiting for the service to exit", nil, "running", wait.Running())
			} else {
				// 所有服务已退出
				log.ZInfo(ctx, "all server exit done")
				return exitCause
			}
		}
	}
}

// putCmd 注册一个服务命令到命令管理器。
//
// 该函数是一个泛型函数，用于注册服务启动函数。它会：
//  1. 从函数指针中提取服务名称
//  2. 创建一个包装函数，负责解析配置并调用服务启动函数
//  3. 将包装函数添加到命令管理器
//
// 类型参数：
//   - C: 服务配置类型
//
// 参数：
//   - cmd: 命令管理器
//   - block: 是否为阻塞服务
//   - fn: 服务启动函数，接收上下文、配置、服务发现注册器和 gRPC 服务注册器
func putCmd[C any](cmd *cmds, block bool, fn func(ctx context.Context, config *C, client discovery.SvcDiscoveryRegistry, server grpc.ServiceRegistrar) error) {
	// 从函数指针中提取服务名称
	name := path.Base(runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name())
	if index := strings.Index(name, "."); index >= 0 {
		name = name[:index]
	}

	// 添加服务命令
	cmd.add(name, block, func(ctx context.Context) error {
		var conf C
		// 解析服务配置
		if err := cmd.parseConf(&conf); err != nil {
			return err
		}
		// 调用服务启动函数
		return fn(ctx, &conf, standalone.GetSvcDiscoveryRegistry(), standalone.GetServiceRegistrar())
	})
}

// cmdManger 管理阻塞服务的生命周期。
//
// 该结构体用于跟踪正在运行的阻塞服务，并在服务退出时通知主流程。
type cmdManger struct {
	lock  sync.Mutex          // 保护并发访问的互斥锁
	done  chan struct{}       // 服务退出通知通道
	count int                 // 正在运行的服务数量
	names map[string]struct{} // 正在运行的服务名称集合
}

// Start 标记一个服务开始运行。
//
// 该方法在服务启动时调用，用于跟踪正在运行的服务。
// 如果服务名称已存在，会触发 panic。
//
// 参数：
//   - name: 服务名称
func (x *cmdManger) Start(name string) {
	x.lock.Lock()
	defer x.lock.Unlock()
	if x.names == nil {
		x.names = make(map[string]struct{})
	}
	if x.done == nil {
		x.done = make(chan struct{}, 1)
	}
	// 检查服务名称是否重复
	if _, ok := x.names[name]; ok {
		panic(fmt.Errorf("cmd %s already exists", name))
	}
	x.count++
	x.names[name] = struct{}{}
}

// Shutdown 标记一个服务已关闭。
//
// 该方法在服务退出时调用，用于更新正在运行的服务列表。
// 当所有服务都退出时，会关闭 done 通道通知主流程。
// 如果服务名称不存在，会触发 panic。
//
// 参数：
//   - name: 服务名称
func (x *cmdManger) Shutdown(name string) {
	x.lock.Lock()
	defer x.lock.Unlock()
	// 检查服务名称是否存在
	if _, ok := x.names[name]; !ok {
		panic(fmt.Errorf("cmd %s not exists", name))
	}
	delete(x.names, name)
	x.count--
	// 如果所有服务都已退出，关闭 done 通道
	if x.count == 0 {
		close(x.done)
	} else {
		// 通知主流程有服务退出
		select {
		case x.done <- struct{}{}:
		default:
		}
	}
}

// Wait 返回一个通道，用于等待所有服务退出。
//
// 返回值：
//   - <-chan struct{}: 服务退出通知通道，当所有服务退出时该通道会被关闭
func (x *cmdManger) Wait() <-chan struct{} {
	x.lock.Lock()
	defer x.lock.Unlock()
	// 如果没有正在运行的服务，返回一个已关闭的通道
	if x.count == 0 || x.done == nil {
		tmp := make(chan struct{})
		close(tmp)
		return tmp
	}
	return x.done
}

// Running 返回当前正在运行的服务名称列表。
//
// 返回值：
//   - []string: 正在运行的服务名称列表
func (x *cmdManger) Running() []string {
	x.lock.Lock()
	defer x.lock.Unlock()
	names := make([]string, 0, len(x.names))
	for name := range x.names {
		names = append(names, name)
	}
	return names
}

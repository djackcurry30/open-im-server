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

// 初始化函数 - 在main函数执行前执行
// 功能：设置独立模式和注册Prometheus指标
func init() {
	config.SetStandalone()
	prommetrics.RegistryAll()
}

// 主函数 - OpenIM服务器启动入口
// 功能：解析命令行参数，注册服务，启动所有组件
func main() {
	var configPath string
	// 解析命令行参数，获取配置文件路径
	flag.StringVar(&configPath, "c", "", "config path")
	flag.Parse()
	// 验证配置路径是否为空
	if configPath == "" {
		_, _ = fmt.Fprintln(os.Stderr, "配置路径为空")
		os.Exit(1)
		return
	}
	// 创建命令管理器
	cmd := newCmds(configPath)
	// 注册各种服务启动函数
	// false表示非阻塞服务，true表示阻塞服务
	putCmd(cmd, false, auth.Start)        // 认证服务
	putCmd(cmd, false, conversation.Start) // 会话服务
	putCmd(cmd, false, relation.Start)    // 关系服务
	putCmd(cmd, false, group.Start)       // 群组服务
	putCmd(cmd, false, msg.Start)         // 消息服务
	putCmd(cmd, false, third.Start)       // 第三方服务
	putCmd(cmd, false, user.Start)        // 用户服务
	putCmd(cmd, false, push.Start)        // 推送服务
	putCmd(cmd, true, msggateway.Start)   // 消息网关服务
	putCmd(cmd, true, msgtransfer.Start)  // 消息传输服务
	putCmd(cmd, true, api.Start)          // API服务
	putCmd(cmd, true, cron.Start)         // 定时任务服务
	
	// 创建上下文，启动所有服务
	ctx := context.Background()
	if err := cmd.run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "服务器退出: %s", err)
		os.Exit(1)
		return
	}
}

// newCmds 创建命令管理器实例
// 参数：confPath - 配置文件路径
// 返回值：cmds指针
func newCmds(confPath string) *cmds {
	return &cmds{confPath: confPath}
}

// cmdName 单个命令结构体
// 包含命令名称、执行函数和是否阻塞标识
type cmdName struct {
	Name  string                     // 命令名称
	Func  func(ctx context.Context) error // 命令执行函数
	Block bool                       // 是否阻塞
}

// cmds 命令管理器结构体
// 管理所有服务启动命令和配置
type cmds struct {
	confPath string                    // 配置文件路径
	cmds     []cmdName                 // 命令列表
	config   config.AllConfig          // 所有配置
	conf     map[string]reflect.Value  // 配置反射映射
}

// getTypePath 获取类型的路径字符串
// 参数：typ - 反射类型
// 返回值：类型路径
func (x *cmds) getTypePath(typ reflect.Type) string {
	return path.Join(typ.PkgPath(), typ.Name())
}

// initDiscovery 初始化服务发现配置
// 功能：设置独立模式下的服务发现配置
func (x *cmds) initDiscovery() {
	x.config.Discovery.Enable = "standalone"
	vof := reflect.ValueOf(&x.config.Discovery.RpcService).Elem()
	tof := reflect.TypeOf(&x.config.Discovery.RpcService).Elem()
	num := tof.NumField()
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

// initAllConfig 初始化所有配置
// 功能：加载配置文件，解析到对应的结构体字段
func (x *cmds) initAllConfig() error {
	x.conf = make(map[string]reflect.Value)
	vof := reflect.ValueOf(&x.config).Elem()
	num := vof.NumField()
	for i := 0; i < num; i++ {
		field := vof.Field(i)
		// 处理指针类型，获取实际值
		for ptr := true; ptr; {
			if field.Kind() == reflect.Ptr {
				field = field.Elem()
			} else {
				ptr = false
			}
		}
		// 存储类型路径到反射值的映射
		x.conf[x.getTypePath(field.Type())] = field
		val := field.Addr().Interface()
		// 获取配置文件名
		name := val.(interface{ GetConfigFileName() string }).GetConfigFileName()
		// 读取配置文件
		confData, err := os.ReadFile(filepath.Join(x.confPath, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		// 解析YAML配置到结构体
		v := viper.New()
		v.SetConfigType("yaml")
		if err := v.ReadConfig(bytes.NewReader(confData)); err != nil {
			return err
		}
		opt := func(conf *mapstructure.DecoderConfig) {
			conf.TagName = config.StructTagName
		}
		if err := v.Unmarshal(val, opt); err != nil {
			return err
		}
	}
	// 初始化服务发现
	x.initDiscovery()
	// 启用Redis
	x.config.Redis.Disable = false
	// 初始化本地缓存配置
	x.config.LocalCache = config.LocalCache{}
	// 初始化通知配置
	config.InitNotification(&x.config.Notification)
	return nil
}

// parseConf 解析配置到结构体
// 参数：conf - 配置结构体指针
// 返回值：错误信息
func (x *cmds) parseConf(conf any) error {
	vof := reflect.ValueOf(conf)
	// 处理指针类型，获取实际值
	for {
		if vof.Kind() == reflect.Ptr {
			vof = vof.Elem()
		} else {
			break
		}
	}
	tof := vof.Type()
	numField := vof.NumField()
	for i := 0; i < numField; i++ {
		typeField := tof.Field(i)
		if !typeField.IsExported() {
			continue
		}
		field := vof.Field(i)
		pkt := x.getTypePath(field.Type())
		val, ok := x.conf[pkt]
		if !ok {
			// 处理特殊类型的配置
			switch field.Interface().(type) {
			case config.Index:
			case config.Path:
				field.SetString(x.confPath)
			case config.AllConfig:
				field.Set(reflect.ValueOf(x.config))
			case *config.AllConfig:
				field.Set(reflect.ValueOf(&x.config))
			default:
				return fmt.Errorf("配置字段 %s %s 未找到", vof.Type().Name(), typeField.Name)
			}
			continue
		}
		field.Set(val)
	}
	return nil
}

// add 添加命令到命令列表
// 参数：
//   - name: 命令名称
//   - block: 是否阻塞
//   - fn: 命令执行函数
func (x *cmds) add(name string, block bool, fn func(ctx context.Context) error) {
	x.cmds = append(x.cmds, cmdName{Name: name, Block: block, Func: fn})
}

// initLog 初始化日志配置
// 返回值：错误信息
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

// run 运行所有命令
// 参数：ctx - 上下文
// 返回值：错误信息
func (x *cmds) run(ctx context.Context) error {
	if len(x.cmds) == 0 {
		return fmt.Errorf("没有命令可运行")
	}
	// 初始化所有配置
	if err := x.initAllConfig(); err != nil {
		return err
	}
	// 初始化日志
	if err := x.initLog(); err != nil {
		return err
	}

	// 创建可取消的上下文
	ctx, cancel := context.WithCancelCause(ctx)

	// 监听上下文完成事件
	go func() {
		<-ctx.Done()
		log.ZError(ctx, "服务上下文退出原因", context.Cause(ctx))
	}()

	// 启动Prometheus监控
	if prometheus := x.config.API.Prometheus; prometheus.Enable {
		var (
			port int
			err  error
		)
		if !prometheus.AutoSetPorts {
			port, err = datautil.GetElemByIndex(prometheus.Ports, 0)
			if err != nil {
				return err
			}
		}
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return fmt.Errorf("Prometheus监听端口 %d 错误: %w", port, err)
		}
		defer listener.Close()
		log.ZDebug(ctx, "Prometheus启动", "地址", listener.Addr())
		go func() {
			err := prommetrics.Start(listener)
			if err == nil {
				err = fmt.Errorf("http done")
			}
			cancel(fmt.Errorf("Prometheus %w", err))
		}()
	}

	// 监听系统信号
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
		select {
		case <-ctx.Done():
			return
		case val := <-sigs:
			log.ZDebug(ctx, "收到信号", "信号", val.String())
			cancel(fmt.Errorf("信号 %s", val.String()))
		}
	}()

	// 启动非阻塞服务
	for i := range x.cmds {
		cmd := x.cmds[i]
		if cmd.Block {
			continue
		}
		// 执行非阻塞服务启动函数
		if err := cmd.Func(ctx); err != nil {
			cancel(fmt.Errorf("服务 %s 退出: %w", cmd.Name, err))
			return err
		}
		go func() {
			if cmd.Block {
				cancel(fmt.Errorf("服务 %s 退出", cmd.Name))
			}
		}()
	}

	// 启动阻塞服务
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
				cancel(fmt.Errorf("服务 %s 退出: %w", cmd.Name, err))
				return
			}
			cancel(fmt.Errorf("服务 %s 退出", cmd.Name))
		}()
	}
	// 等待上下文完成
	<-ctx.Done()
	exitCause := context.Cause(ctx)
	log.ZWarn(ctx, "服务关闭通知", exitCause)
	// 等待所有服务优雅关闭
	done := wait.Wait()
	timeout := time.NewTimer(time.Second * 10)
	defer timeout.Stop()
	for {
		select {
		case <-timeout.C:
			log.ZWarn(ctx, "服务退出超时", nil, "运行中", wait.Running())
			return exitCause
		case _, ok := <-done:
			if ok {
				log.ZWarn(ctx, "等待服务退出", nil, "运行中", wait.Running())
			} else {
				log.ZInfo(ctx, "所有服务退出完成")
				return exitCause
			}
		}
	}
}

// putCmd 注册服务启动函数到命令管理器
// 参数：
//   - cmd: 命令管理器
//   - block: 是否阻塞
//   - fn: 服务启动函数
func putCmd[C any](cmd *cmds, block bool, fn func(ctx context.Context, config *C, client discovery.SvcDiscoveryRegistry, server grpc.ServiceRegistrar) error) {
	// 获取函数名作为服务名
	name := path.Base(runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name())
	if index := strings.Index(name, "."); index >= 0 {
		name = name[:index]
	}
	// 添加到命令列表
	cmd.add(name, block, func(ctx context.Context) error {
		var conf C
		// 解析配置
		if err := cmd.parseConf(&conf); err != nil {
			return err
		}
		// 调用服务启动函数
		return fn(ctx, &conf, standalone.GetSvcDiscoveryRegistry(), standalone.GetServiceRegistrar())
	})
}

// cmdManger 命令管理器
// 功能：管理服务的启动和关闭
type cmdManger struct {
	lock  sync.Mutex          // 互斥锁
	done  chan struct{}       // 完成通知通道
	count int                 // 运行中的服务数量
	names map[string]struct{} // 运行中的服务名称
}

// Start 启动服务
// 参数：name - 服务名称
func (x *cmdManger) Start(name string) {
	x.lock.Lock()
	defer x.lock.Unlock()
	if x.names == nil {
		x.names = make(map[string]struct{})
	}
	if x.done == nil {
		x.done = make(chan struct{}, 1)
	}
	if _, ok := x.names[name]; ok {
		panic(fmt.Errorf("服务 %s 已存在", name))
	}
	x.count++
	x.names[name] = struct{}{}
}

// Shutdown 关闭服务
// 参数：name - 服务名称
func (x *cmdManger) Shutdown(name string) {
	x.lock.Lock()
	defer x.lock.Unlock()
	if _, ok := x.names[name]; !ok {
		panic(fmt.Errorf("服务 %s 不存在", name))
	}
	delete(x.names, name)
	x.count--
	if x.count == 0 {
		close(x.done)
	} else {
		select {
		case x.done <- struct{}{}:
		default:
		}
	}
}

// Wait 等待所有服务关闭
// 返回值：完成通知通道
func (x *cmdManger) Wait() <-chan struct{} {
	x.lock.Lock()
	defer x.lock.Unlock()
	if x.count == 0 || x.done == nil {
		tmp := make(chan struct{})
		close(tmp)
		return tmp
	}
	return x.done
}

// Running 获取运行中的服务名称
// 返回值：服务名称列表
func (x *cmdManger) Running() []string {
	x.lock.Lock()
	defer x.lock.Unlock()
	names := make([]string, 0, len(x.names))
	for name := range x.names {
		names = append(names, name)
	}
	return names
}
